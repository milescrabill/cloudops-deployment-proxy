package proxyservice

import (
	"log"
	"net/http"
)

type DockerHubWebhookHandler struct {
	Jenkins         *Jenkins
	ValidNameSpaces map[string]bool
}

func NewDockerHubWebhookHandler(jenkins *Jenkins, nameSpaces ...string) *DockerHubWebhookHandler {
	validNameSpaces := make(map[string]bool)
	for _, nameSpace := range nameSpaces {
		validNameSpaces[nameSpace] = true
	}
	return &DockerHubWebhookHandler{
		Jenkins:         jenkins,
		ValidNameSpaces: validNameSpaces,
	}
}

func (d *DockerHubWebhookHandler) isValidNamespace(nameSpace string) bool {
	return d.ValidNameSpaces[nameSpace]
}

func (d *DockerHubWebhookHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != "POST" {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	log.Printf("Received dockerhub request from: %s", req.RemoteAddr)

	hookData, err := NewDockerHubWebhookDataFromRequest(req)
	if err != nil {
		log.Printf("Error parsing request: %v", err)
		http.Error(w, "Internal Service Error", http.StatusInternalServerError)
		return
	}

	if !d.isValidNamespace(hookData.Repository.Namespace) {
		log.Printf("Invalid Namespace: %s", hookData.Repository.Namespace)
		http.Error(w, "Invalid Namespace", http.StatusUnauthorized)
		return
	}

	if err := hookData.Callback(NewSuccessCallbackData()); err != nil {
		log.Printf("Callback error: %v", err)
		http.Error(w, "Request could not be validated", http.StatusUnauthorized)
		return
	}

	rawJSON, err := hookData.rawJSON()
	if err != nil {
		log.Printf(err.Error())
		http.Error(w, "Internal Service Error", http.StatusInternalServerError)
		return
	}

	err = d.Jenkins.TriggerJenkinsJob(
		"dockerhub",
		hookData.Repository.Name,
		hookData.Repository.Namespace,
		hookData.PushData.Tag,
		rawJSON,
	)
	if err != nil {
		log.Printf("Error triggering jenkins: %v", err)
		http.Error(w, "Internal Service Error", http.StatusInternalServerError)
		return
	}

	w.Write([]byte("OK"))
}

type GcrWebhookHandler struct {
	Jenkins      *Jenkins
	PubSubSecret string
}

func (d *GcrWebhookHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != "POST" {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	queryParams := req.URL.Query()

	if queryParams.Get("secret") != d.PubSubSecret {
		log.Printf("Received request with invalid secret")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	hookData, err := NewGcrWebhookDataFromRequest(req)
	if err != nil {
		log.Printf("Error parsing request: %v", err)
		http.Error(w, "Internal Service Error", http.StatusInternalServerError)
		return
	}

	if !hookData.isValid() {
		log.Printf("Received invalid request: %v", err)
		http.Error(w, "Internal Service Error", http.StatusInternalServerError)
		return
	}

	rawJSON, err := hookData.rawJSON()
	if err != nil {
		log.Printf(err.Error())
		http.Error(w, "Internal Service Error", http.StatusInternalServerError)
		return
	}

	err = d.Jenkins.TriggerJenkinsJob(
		"gcr",
		hookData.getRepositoryName(),
		hookData.getRepositoryDomain(),
		hookData.getImageTagOrDigest(),
		rawJSON,
	)

	if err != nil {
		log.Printf("Error triggering jenkins: %v", err)
		http.Error(w, "Internal Service Error", http.StatusInternalServerError)
		return
	}

	w.Write([]byte("OK"))
}

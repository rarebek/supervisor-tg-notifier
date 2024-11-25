package supervisor

import (
	"encoding/base64"
	"fmt"
	"log"
	"net/http"

	"github.com/kolo/xmlrpc"
	"github.com/rarebek/supervisor-tg-notifier/pkg/models"
)

// BasicAuth holds credentials
type BasicAuth struct {
	Username string
	Password string
}

type Client struct {
	xmlrpc *xmlrpc.Client
	auth   *BasicAuth
}

// encodeBasicAuth creates Base64 encoded auth string
func encodeBasicAuth(username, password string) string {
	auth := fmt.Sprintf("%s:%s", username, password)
	return base64.StdEncoding.EncodeToString([]byte(auth))
}

// NewClient creates supervisor client with basic auth
func NewClient(serverURL string, auth *BasicAuth) (*Client, error) {
	// Create custom transport
	transport := &http.Transport{}

	// Create custom RoundTripper to add auth header
	authTransport := &authRoundTripper{
		base: transport,
		auth: auth,
	}

	// Create XML-RPC client with auth transport
	client, err := xmlrpc.NewClient(serverURL, authTransport)
	if err != nil {
		return nil, fmt.Errorf("failed to create XML-RPC client: %w", err)
	}

	return &Client{
		xmlrpc: client,
		auth:   auth,
	}, nil
}

// authRoundTripper implements http.RoundTripper
type authRoundTripper struct {
	base http.RoundTripper
	auth *BasicAuth
}

// RoundTrip adds auth header to requests
func (t *authRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.auth != nil {
		req.Header.Set("Authorization", "Basic "+encodeBasicAuth(t.auth.Username, t.auth.Password))
	}
	return t.base.RoundTrip(req)
}

func (c *Client) Close() {
	c.xmlrpc.Close()
}

func (c *Client) GetAllProcesses() ([]models.Process, error) {
	var processesRaw []map[string]interface{}
	err := c.xmlrpc.Call("supervisor.getAllProcessInfo", nil, &processesRaw)
	if err != nil {
		return nil, err
	}

	processes := make([]models.Process, len(processesRaw))
	for i, p := range processesRaw {
		name, ok := p["name"].(string)
		if !ok {
			name = "Unknown"
		}

		state, ok := p["statename"].(string)
		if !ok {
			state = "Unknown"
		}

		group, ok := p["group"].(string)
		if !ok {
			group = "Unknown"
		}

		description, ok := p["description"].(string)
		if !ok {
			description = "No description available"
		}

		processes[i] = models.Process{
			Name:        name,
			State:       state,
			Description: description,
			Group:       group,
		}
	}

	return processes, nil
}

func (c *Client) StartProcess(processName string) error {
	var result bool
	err := c.xmlrpc.Call("supervisor.startProcess", []interface{}{processName}, &result)
	if err != nil {
		log.Printf("Error starting process %s: %v", processName, err)
	}
	return err
}

func (c *Client) StopProcess(processName string) error {
	var result bool
	err := c.xmlrpc.Call("supervisor.stopProcess", []interface{}{processName}, &result)
	if err != nil {
		log.Printf("Error stopping process %s: %v", processName, err)
	}
	return err
}

func (c *Client) GetProcessInfo(processName string) (models.Process, error) {
	var processRaw map[string]interface{}
	err := c.xmlrpc.Call("supervisor.getProcessInfo", []interface{}{processName}, &processRaw)
	if err != nil {
		return models.Process{}, fmt.Errorf("failed to get process info: %w", err)
	}

	name, ok := processRaw["name"].(string)
	if !ok {
		name = "Unknown"
	}

	state, ok := processRaw["statename"].(string)
	if !ok {
		state = "Unknown"
	}

	group, ok := processRaw["group"].(string)
	if !ok {
		group = "Unknown"
	}

	description, ok := processRaw["description"].(string)
	if !ok {
		description = "No description available"
	}

	return models.Process{
		Name:        name,
		State:       state,
		Description: description,
		Group:       group,
	}, nil
}

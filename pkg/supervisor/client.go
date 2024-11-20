package supervisor

import (
	"log"

	"github.com/kolo/xmlrpc"
	"github.com/rarebek/supervisor-tg-notifier/pkg/models"
)

type Client struct {
	xmlrpc *xmlrpc.Client
}

func NewClient(serverURL string) (*Client, error) {
	client, err := xmlrpc.NewClient(serverURL, nil)
	if err != nil {
		return nil, err
	}
	return &Client{xmlrpc: client}, nil
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

		description, ok := p["description"].(string)
		if !ok {
			description = "No description available"
		}

		processes[i] = models.Process{
			Name:        name,
			State:       state,
			Description: description,
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

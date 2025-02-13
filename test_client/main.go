package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"
)

type VeryFirstEvent struct {
	Event           string
	MessageEndpoint string
	ClientID        string
}

type InitializeMethodResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Result  Result `json:"result"`
}

type Result struct {
	ProtocolVersion string       `json:"protocolVersion"`
	Capabilities    Capabilities `json:"capabilities"`
	ServerInfo      ServerInfo   `json:"serverInfo"`
}

type Capabilities struct {
	Logging struct{} `json:"logging"`
	Prompts struct {
		ListChanged bool `json:"listChanged"`
	} `json:"prompts"`
	Resources struct {
		ListChanged bool `json:"listChanged"`
		Subscribe   bool `json:"subscribe"`
	} `json:"resources"`
	Tools struct {
		ListChanged bool `json:"listChanged"`
	} `json:"tools"`
}

type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func main() {
	resp, err := http.Get("http://localhost:8080/events")
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	var veryFirstEvent VeryFirstEvent
	startTime := time.Now()

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event:") {
			veryFirstEvent.Event = strings.TrimSpace(line[len("event:"):])
		} else if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(line[len("data:"):])
			if strings.HasPrefix(data, "{") && strings.HasSuffix(data, "}") {
				var initResponse InitializeMethodResponse
				if err := json.Unmarshal([]byte(data), &initResponse); err != nil {
					fmt.Printf("Error decoding response: %v\n", err)
					continue
				}
				fmt.Printf("InitializeMethodResponse: %+v\n", initResponse)
			} else {
				veryFirstEvent.MessageEndpoint = data
			}
		} else if strings.HasPrefix(line, ":ping") {
			// Handle ping response, no action needed
			fmt.Println("Received ping")
		}

		if veryFirstEvent.MessageEndpoint != "" {
			fmt.Printf("EventData: %+v\n", veryFirstEvent)

			if time.Since(startTime) > 100*time.Second {
				fmt.Println("Failed to identify Client ID within 10 seconds")
				continue
			}

			go func(endpoint string) {
				// Prepare the JSON-RPC request payload
				payload := map[string]interface{}{
					"jsonrpc": "2.0",
					"id":      1,
					"method":  "initialize",
					"params": map[string]interface{}{
						"protocolVersion": "2024-11-05",
						"capabilities": map[string]interface{}{
							"tools": map[string]bool{
								"listChanged": true,
							},
							"sampling": map[string]interface{}{},
						},
						"clientInfo": map[string]string{
							"name":    "ExampleClient",
							"version": "1.0.0",
						},
					},
				}

				payloadBytes, err := json.Marshal(payload)
				if err != nil {
					fmt.Printf("Error marshaling payload: %v\n", err)
					return
				}

				// Send the POST request to the MessageEndpoint
				postResp, err := http.Post(endpoint, "application/json", bytes.NewBuffer(payloadBytes))
				if err != nil {
					fmt.Printf("Error sending POST request: %v\n", err)
					return
				}
				defer postResp.Body.Close()

				// Check if the response body is empty
				bodyBytes, err := ioutil.ReadAll(postResp.Body)
				if err != nil {
					fmt.Printf("Error reading response body: %v\n", err)
					return
				}
				if len(bodyBytes) == 0 {
					fmt.Println("Empty response body")
				} else {
					// Parse the response into InitializeMethodResponse struct
					var initResponse InitializeMethodResponse
					if err := json.Unmarshal(bodyBytes, &initResponse); err != nil {
						fmt.Printf("Error decoding response: %v\n", err)
						return
					}

					// Print the parsed response
					fmt.Printf("InitializeMethodResponse: %+v\n", initResponse)
				}

				// Send the initialized notification
				notificationPayload := map[string]interface{}{
					"jsonrpc": "2.0",
					"method":  "notifications/initialized",
				}

				notificationBytes, err := json.Marshal(notificationPayload)
				if err != nil {
					fmt.Printf("Error marshaling notification payload: %v\n", err)
					return
				}

				notificationResp, err := http.Post(endpoint, "application/json", bytes.NewBuffer(notificationBytes))
				if err != nil {
					fmt.Printf("Error sending notification: %v\n", err)
					return
				}
				defer notificationResp.Body.Close()

				if notificationResp.StatusCode == http.StatusOK {
					fmt.Println("Client is connected and ready to send messages")
				} else {
					fmt.Printf("Failed to send initialized notification: %v\n", notificationResp.Status)
				}
			}(veryFirstEvent.MessageEndpoint)

			// Reset the MessageEndpoint to continue processing new events
			veryFirstEvent.MessageEndpoint = ""
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf("Error reading response: %v\n", err)
	}
}

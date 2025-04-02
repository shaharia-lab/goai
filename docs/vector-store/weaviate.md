# Weaviate Vector Database

## Run Embedded Weaviate

<!-- markdownlint-disable -->
```go
package main

import (
	"github.com/shaharia-lab/goai/vectordb/weaviate"
	"log"
)

func main() {
	logger := log.New(log.Writer(), "[main] ", log.LstdFlags)

	options := weaviate.Options{
		// PersistenceDataPath: "/path/to/custom/data", // Optional: Custom data path
		// BinaryPath:          "/path/to/custom/bin",  // Optional: Custom binary cache
		// Version:             "1.25.3",               // Optional: Specific version
		// Version:             "latest",               // Optional: Use latest version
		// Port:                9090,                   // Optional: Custom HTTP port
		// GRPCPort:            50090,                  // Optional: Custom gRPC port
		// Hostname:            "0.0.0.0",              // Optional: Bind to all interfaces
		AdditionalEnvVars: map[string]string{ // Optional: Add/override env vars
			// "DEFAULT_VECTORIZER_MODULE": "text2vec-huggingface",
			// "HUGGINGFACE_APIKEY": "your_hf_key_here", // Example
			"LOG_LEVEL": "debug",
		},
	}

	db, err := weaviate.AsEmbedded(options)
	if err != nil {
		logger.Fatalf("Failed to initialize embedded Weaviate: %v", err)
	}

	// Option 1: Start and manage manually
	/*
		err = db.Start()
		if err != nil {
			logger.Fatalf("Failed to start embedded Weaviate: %v", err)
		}

		logger.Println("Embedded Weaviate started successfully!")

		// Your application logic here...
		// You can connect to Weaviate at options.Hostname:options.Port (HTTP)
		// and options.Hostname:options.GRPCPort (gRPC) using a standard Weaviate client.
		logger.Println("Running application logic for 15 seconds...")
		time.Sleep(15 * time.Second)


		logger.Println("Stopping embedded Weaviate...")
		err = db.Stop()
		if err != nil {
			logger.Fatalf("Failed to stop embedded Weaviate: %v", err)
		}
		logger.Println("Embedded Weaviate stopped.")
	*/

	// Option 2: Start and block until Ctrl+C (SIGINT/SIGTERM)
	logger.Println("Starting embedded Weaviate and watching for signals...")
	err = db.StartAndWatch() // This will block until signal or startup error
	if err != nil {
		// This error usually comes from Stop() if it fails during shutdown
		logger.Printf("Shutdown completed with error: %v", err)
	} else {
		logger.Println("Shutdown completed successfully.")
	}
}
```
<!-- markdownlint-enable -->
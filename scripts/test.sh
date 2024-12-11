#!/bin/bash

# Start Docker containers for Prometheus and Grafana
docker-compose up -d prometheus grafana

# Wait for services to be ready
echo "Waiting for services to start..."
sleep 5

# Run the application
go run main.go start "wss://ws.kraken.com" --pair="XBT/USD" 
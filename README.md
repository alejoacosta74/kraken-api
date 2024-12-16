# 🦑 Kraken OrderBook Streaming Engine

A high-performance, real-time order book streaming engine that connects to Kraken's WebSocket API, processes market data, and distributes it through Kafka while providing comprehensive metrics via Prometheus and Grafana.

## 🌟 Features

- Real-time order book data streaming from [Kraken's WebSocket API v2](https://docs.kraken.com/api/docs/guides/spot-ws-book-v2/)
- Efficient message processing with concurrent handlers
- Event-driven architecture using a pub/sub pattern
- Scalable Kafka message distribution with producer pooling
- Comprehensive metrics collection and visualization
- Thread-safe operations with proper resource management

## 🏗️ Architecture

```ascii
                                Kraken WebSocket API
                                          │
                                          ▼
┌─────────────────────────────────────────────────────────────────┐
│                       WebSocket Client                          │
│                        (client.go)                              │ 
└───────────────────────────────┬─────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                         Dispatcher                              │
│                      (dispatcher.go)                            │
└───────────┬─────────────────────┬────────────────┬───────-──────┘
            │                     │                │
            ▼                     ▼                ▼
┌───────────────────┐   ┌─────────────────┐   ┌──────────────┐
│  Snapshot Handler │   │ Update Handler  │   │Debug Handler │
│                   │   │                 │   │              │
└───────┬───┬───────┘   └────────┬─┬────-─┘   └─────┬────────┘
        │   │                    │ │                │
        │   │                    │ │                │
    ┌───┘   │              ┌─────┘ │            Publishes
    │       │              │       │                │
    │       └──────────────┼───────┼────────────────┘
    │                      │       │
    ▼                      ▼       │
┌─────────────────────────────┐    │     ┌─────────────────────────┐
│      Producer Pool          │    │     │       Event Bus         │
│     (producer_pool.go)      │    └────►│       (bus.go)          │
└──────────────┬────────────-─┘          └────────────┬────────────┘
               │                      				  │
               │                      				  │
               │                     			   	  │
               ▼                   					  ▼
┌─────────────────────────────┐    ┌─────────────────────────────┐
│                             │    │     Metrics Recorder        │
│      Kafka Cluster          │    │      (recorder.go)          │
│        (Docker)             │    │                             │
└─────────────────────────────┘    └─────────────────────────────┘
                                              │
                                              ▼
                                   ┌─────────────────────────┐
                                   │  Prometheus & Grafana   │
                                   │       (Docker)          │
                                   └─────────────────────────┘
```

## 🔍 Component Overview

- **WebSocket Client** (`client.go`): Manages real-time connection with Kraken's WebSocket API
- **Dispatcher** (`dispatcher.go`): Routes messages to appropriate handlers based on message type
- **Event Bus**: Implements pub/sub pattern for system-wide event distribution
- **Kafka Producer Pool**: Manages a pool of producers for efficient message distribution
- **Metrics Recorder**: Collects and exposes metrics for monitoring and analysis

## 🚀 Prerequisites

- Docker and Docker Compose for running:
  - Prometheus and Grafana containers
  - Kafka cluster environment
- Go 1.21 or higher
- Access to Kraken's WebSocket API

## 📦 Installation

1. Clone the repository:

2. Install dependencies:
3. 
4. Start the required Docker containers:

## 🛠️ Configuration

The application can be configured through command-line flags or environment variables:

```bash
./kraken-orderbook-engine start wss://ws.kraken.com \
--pair="ETH/USD" \
--metrics-addr=":2112" \
--kafka-cluster-addresses="localhost:9092" \
--kafka-producer-pool-size=5
```

## 📊 Monitoring

- Prometheus metrics available at `http://localhost:2112/metrics`
- Grafana dashboards accessible at `http://localhost:3000`

## 🔄 Message Flow

1. WebSocket client connects to Kraken and subscribes to order book updates
2. Messages are received and passed to the dispatcher
3. Dispatcher routes messages to appropriate handlers
4. Handlers process messages and publish events to the event bus
5. Kafka producers distribute processed messages to configured topics
6. Metrics are collected and exposed for monitoring

## 📝 Types and Models

The `kraken` package defines all necessary types for handling:
- Book snapshots and updates
- System status messages
- Subscription requests and responses
- Heartbeat messages

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## 📄 License

This project is licensed under the MIT License - see the LICENSE file for details.

## 🙏 Acknowledgments

- Kraken API team for their comprehensive WebSocket API documentation
- The Go community for excellent tooling and libraries
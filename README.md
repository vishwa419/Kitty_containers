# 🐱 Kitten Container Orchestration

**Lightweight container orchestration with AI-powered natural language deployment**

Kitten is a Docker-like container orchestration system written in Go, featuring three powerful ways to deploy: direct JSON API, CLI configuration files, and an AI-powered natural language interface.

---

## ✨ Features

- 🚀 **Three deployment methods**: JSON API, CLI, or natural language (AI-powered)
- 🐳 **Container orchestration**: Multi-container deployments with dependency management
- 🌐 **Network management**: Bridge networks with custom subnets and inter-container communication
- 🔄 **Restart policies**: Auto-restart containers with `always` or `on-failure` policies
- 📦 **Port mapping**: Expose container ports to host
- 🔗 **Dependency resolution**: Start containers in correct order based on dependencies
- 🤖 **AI Gateway**: Describe what you want in plain English, powered by OpenAI

---

## 🏗️ Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        User Interface                        │
│  ┌──────────┐    ┌──────────┐    ┌────────────────────┐    │
│  │   CLI    │    │ JSON API │    │   AI Gateway 🤖    │    │
│  │          │    │          │    │  (Natural Language) │    │
│  └────┬─────┘    └────┬─────┘    └─────────┬──────────┘    │
└───────┼──────────────┼────────────────────┼─────────────────┘
        │              │                     │
        └──────────────┴─────────────────────┘
                       │
        ┌──────────────▼───────────────┐
        │   Kitten API Server (Go)     │
        │         Port 8080             │
        │  ┌────────────────────────┐  │
        │  │   Container Manager    │  │
        │  │  - Spawn containers    │  │
        │  │  - Manage networks     │  │
        │  │  - Handle dependencies │  │
        │  └────────────────────────┘  │
        └──────────────┬───────────────┘
                       │
        ┌──────────────▼───────────────┐
        │      Running Containers       │
        │    (Isolated with Namespaces) │
        └───────────────────────────────┘
```

**Components:**
- **Kitten API Server** (`main.go`): Go-based HTTP API server on port 8080
- **Container Manager** (`manager.go`): Orchestrates multiple containers, networks, and dependencies
- **AI Gateway** (`meow.py`): Python FastAPI server on port 8000 with OpenAI integration

---

## 🚀 Quick Start

### Prerequisites

- **Go 1.18+** (for Kitten API server)
- **Python 3.8+** (for AI Gateway)
- **Root/sudo access** (required for namespaces)
- **OpenAI API Key** (optional, only for AI Gateway)

### Installation

```bash
# Clone the repository
git clone https://github.com/yourusername/kitten.git
cd kitten

# Build the Go server
go build -o kitten-server main.go

# Install Python dependencies
pip install fastapi uvicorn httpx openai pydantic
```

### Start the Servers

```bash
# Terminal 1: Start Kitten API Server (requires root)
sudo ./kitten-server

# Terminal 2: Start AI Gateway (optional)
export OPENAI_API_KEY='your-api-key-here'
python meow.py
```

---

## 📖 Usage

### Method 1: AI Gateway 🤖 (Natural Language)

**The easiest way to deploy!** Just describe what you want in plain English.

1. Start both servers (see Quick Start)
2. Open browser: http://localhost:8000
3. Type what you want to deploy:

**Example prompts:**
- "Deploy nginx web server on port 8080"
- "Run 3 nginx containers behind a load balancer with a shared network"
- "Set up a postgres database and redis cache that can talk to each other"
- "Create a microservices setup with nginx, 2 python API servers, postgres database, and redis"

The AI will generate the configuration, show you what will be deployed, and you can review before confirming!

---

### Method 2: JSON API (Direct HTTP)

Send JSON configurations directly to the Kitten API server.

#### Start a Deployment

```bash
curl -X POST http://localhost:8080/spawn \
  -H "Content-Type: application/json" \
  -d '{
    "version": "1.0",
    "containers": {
      "web": {
        "image": "./rootfs",
        "command": ["/bin/bash", "-c", "echo Hello && sleep 10"],
        "hostname": "web",
        "restart": "on-failure"
      }
    }
  }'
```

**Response:**
```json
{
  "id": "deploy_abc123",
  "message": "Containers are being spawned",
  "status": "starting"
}
```

#### Check Status

```bash
curl http://localhost:8080/status/deploy_abc123
```

#### List All Deployments

```bash
curl http://localhost:8080/list
```

#### Stop a Deployment

```bash
curl -X POST http://localhost:8080/stop/deploy_abc123
```

---

### Method 3: CLI with Configuration File

Create a JSON configuration file and deploy using the CLI.

**config.json:**
```json
{
  "version": "1.0",
  "containers": {
    "web": {
      "image": "nginx:latest",
      "ports": ["8080:80"],
      "hostname": "web",
      "network": "mynet"
    },
    "db": {
      "image": "postgres:latest",
      "environment": {
        "POSTGRES_PASSWORD": "secret",
        "POSTGRES_USER": "admin",
        "POSTGRES_DB": "mydb"
      },
      "network": "mynet"
    }
  },
  "networks": {
    "mynet": {
      "driver": "bridge",
      "subnet": "10.0.0.0/24",
      "gateway": "10.0.0.1"
    }
  }
}
```

**Deploy:**
```bash
sudo ./kitten-server --config config.json
```

---

## 📋 Configuration Reference

### Container Specification

```json
{
  "containers": {
    "container_name": {
      "image": "path/to/rootfs or image:tag",
      "command": ["optional", "command"],
      "hostname": "hostname",
      "workdir": "/path",
      "environment": {
        "KEY": "value"
      },
      "ports": ["8080:80", "3000:3000"],
      "network": "network_name",
      "ip": "10.0.0.10",
      "depends_on": ["other_container"],
      "restart": "always|on-failure|no"
    }
  }
}
```

**Fields:**
- `image` (required): Path to container rootfs or image name
- `command` (optional): Command to run in container
- `hostname` (optional): Container hostname (defaults to container name)
- `workdir` (optional): Working directory (defaults to `/`)
- `environment` (optional): Environment variables as key-value pairs
- `ports` (optional): Port mappings in `host:container` format
- `network` (optional): Network to attach to
- `ip` (optional): Static IP address for container
- `depends_on` (optional): List of containers that must start first
- `restart` (optional): Restart policy (`always`, `on-failure`, or `no`)

### Network Specification

```json
{
  "networks": {
    "network_name": {
      "driver": "bridge|host|none",
      "subnet": "10.0.0.0/24",
      "gateway": "10.0.0.1"
    }
  }
}
```

**Network drivers:**
- `bridge`: Creates a virtual bridge for container communication
- `host`: Uses host network namespace
- `none`: No networking

---

## 💡 Examples

### Example 1: Single Web Server

```json
{
  "version": "1.0",
  "containers": {
    "nginx": {
      "image": "nginx:latest",
      "ports": ["8080:80"],
      "hostname": "web",
      "restart": "always"
    }
  }
}
```

### Example 2: Web + Database with Networking

```json
{
  "version": "1.0",
  "containers": {
    "web": {
      "image": "nginx:latest",
      "ports": ["8080:80"],
      "network": "mynet",
      "depends_on": ["db"]
    },
    "db": {
      "image": "postgres:latest",
      "environment": {
        "POSTGRES_PASSWORD": "secret"
      },
      "network": "mynet",
      "restart": "always"
    }
  },
  "networks": {
    "mynet": {
      "driver": "bridge",
      "subnet": "10.0.0.0/24",
      "gateway": "10.0.0.1"
    }
  }
}
```

### Example 3: Microservices Stack

```json
{
  "version": "1.0",
  "containers": {
    "nginx": {
      "image": "nginx:latest",
      "ports": ["80:80"],
      "network": "microservices",
      "depends_on": ["api1", "api2"]
    },
    "api1": {
      "image": "python:3.11-slim",
      "command": ["python", "-m", "http.server", "8001"],
      "network": "microservices",
      "depends_on": ["redis", "postgres"]
    },
    "api2": {
      "image": "python:3.11-slim",
      "command": ["python", "-m", "http.server", "8002"],
      "network": "microservices",
      "depends_on": ["redis", "postgres"]
    },
    "postgres": {
      "image": "postgres:latest",
      "environment": {
        "POSTGRES_PASSWORD": "secret"
      },
      "network": "microservices",
      "restart": "always"
    },
    "redis": {
      "image": "redis:alpine",
      "network": "microservices",
      "restart": "always"
    }
  },
  "networks": {
    "microservices": {
      "driver": "bridge",
      "subnet": "10.0.0.0/24"
    }
  }
}
```

---

## 🔌 API Reference

### Kitten API Server (Port 8080)

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/spawn` | Create a new deployment from JSON config |
| GET | `/status/{id}` | Get deployment status |
| POST | `/stop/{id}` | Stop a deployment |
| GET | `/list` | List all active deployments |
| GET | `/health` | Health check |

### AI Gateway (Port 8000)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/` | Web UI for natural language deployment |
| POST | `/parse` | Parse natural language to config (returns session_id) |
| POST | `/confirm` | Confirm and deploy configuration |
| GET | `/status/{id}` | Proxy to Kitten API status |
| POST | `/stop/{id}` | Proxy to Kitten API stop |
| GET | `/list` | Proxy to Kitten API list |
| GET | `/health` | Health check for both services |

---

## 🤖 AI Gateway Details

The AI Gateway uses OpenAI's GPT-4 to convert natural language into Kitten configuration JSON.

### How It Works

1. User enters prompt in natural language
2. AI Gateway sends prompt to OpenAI with system instructions
3. OpenAI generates valid Kitten JSON configuration
4. User reviews configuration in the UI
5. On confirmation, configuration is sent to Kitten API
6. Containers are spawned in the background

### Setting Up OpenAI

```bash
# Export your API key
export OPENAI_API_KEY='sk-...'

# Start the gateway
python meow.py
```

### Example AI Prompts

✅ **Good prompts:**
- "Deploy nginx web server on port 8080"
- "Run postgres database with user 'admin' and password 'secret'"
- "Create 3 nginx containers that can talk to each other"
- "Set up wordpress with mysql database"

❌ **Less specific prompts:**
- "Deploy something"
- "Make a server"

**Tip:** The more specific you are about images, ports, and requirements, the better the AI will generate the configuration!

---

## 🛠️ Advanced Topics

### Restart Policies

- **`always`**: Always restart container when it exits
- **`on-failure`**: Only restart on non-zero exit codes
- **`no`**: Never restart (default)

```json
{
  "containers": {
    "db": {
      "image": "postgres:latest",
      "restart": "always"
    }
  }
}
```

### Dependencies

Use `depends_on` to ensure containers start in order:

```json
{
  "containers": {
    "web": {
      "image": "nginx:latest",
      "depends_on": ["db", "cache"]
    },
    "db": { "image": "postgres:latest" },
    "cache": { "image": "redis:alpine" }
  }
}
```

**Start order:** `db` → `cache` → `web`

### Network Modes

- **bridge**: Isolated network with bridge (default)
- **host**: Use host's network directly
- **none**: No networking

```json
{
  "networks": {
    "frontend": {
      "driver": "bridge",
      "subnet": "10.0.1.0/24"
    },
    "backend": {
      "driver": "bridge",
      "subnet": "10.0.2.0/24"
    }
  }
}
```

---

## 🐛 Troubleshooting

### Issue: "Permission denied" when starting server

**Solution:** Kitten requires root privileges for namespace operations.
```bash
sudo ./kitten-server
```

### Issue: "Port already in use"

**Solution:** Change the port or stop the conflicting service.
```bash
# Check what's using the port
sudo lsof -i :8080

# Change Kitten API port
./kitten-server --port 9090
```

### Issue: "OpenAI API error" in AI Gateway

**Solutions:**
- Verify API key: `echo $OPENAI_API_KEY`
- Check API key permissions at platform.openai.com
- Ensure you have sufficient credits

### Issue: "Failed to start container"

**Common causes:**
- Invalid `command` in config
- Missing `image` path
- Circular dependencies
- Network conflicts

**Debug:** Check server logs for detailed error messages.

### Issue: "Network already exists"

**Solution:** Previous deployment may not have cleaned up properly.
```bash
# List existing bridges
ip link show

# Manually delete bridge if needed
sudo ip link delete kitten0
```

---

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/AmazingFeature`)
3. Commit your changes (`git commit -m 'Add some AmazingFeature'`)
4. Push to the branch (`git push origin feature/AmazingFeature`)
5. Open a Pull Request

---

## 📝 License

This project is licensed under the MIT License - see the LICENSE file for details.

---

## 🙏 Acknowledgments

- Inspired by Docker and Kubernetes
- Powered by OpenAI for natural language processing
- Built with Go and Python

---

## 📞 Support

For issues and questions:
- Open an issue on GitHub
- Check existing issues for solutions
- Review the troubleshooting section

---

**Made with 🐱 by the Kitten team**

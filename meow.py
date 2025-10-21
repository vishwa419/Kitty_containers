from fastapi import FastAPI, HTTPException
from fastapi.responses import HTMLResponse
from pydantic import BaseModel
from typing import Optional, Dict, List
import httpx
import json
from datetime import datetime
import uuid
import os
from openai import OpenAI

app = FastAPI(title="Kitten NLP Gateway", version="2.0.0")

# Configuration
KITTEN_API_URL = "http://localhost:8080"
OPENAI_API_KEY = os.getenv("OPENAI_API_KEY")

if not OPENAI_API_KEY:
    print("WARNING: OPENAI_API_KEY not set. Set it with: export OPENAI_API_KEY='your-key'")

# OpenAI Client
client = OpenAI(api_key=OPENAI_API_KEY)

# In-memory storage for pending deployments
pending_deployments = {}

# Request/Response Models


class NLPRequest(BaseModel):
    prompt: str


class ParseResponse(BaseModel):
    session_id: str
    prompt: str
    config: Dict
    explanation: str


class ConfirmRequest(BaseModel):
    session_id: str


class DeploymentResponse(BaseModel):
    id: str
    message: str
    status: str
    config: Dict

# OpenAI-powered NLP Parser


class KittenAIParser:
    """Uses OpenAI to convert natural language to Kitten container configuration"""

    def __init__(self):
        self.system_prompt = """You are an expert in container orchestration and Docker-like configurations.
Your job is to convert natural language requests into JSON configurations for the Kitten container orchestration system.

The configuration format is:
{
  "version": "1.0",
  "containers": {
    "container_name": {
      "image": "image:tag",
      "command": ["optional", "command"],
      "hostname": "hostname",
      "workdir": "/path",
      "environment": {"KEY": "value"},
      "ports": ["hostPort:containerPort"],
      "network": "network_name",
      "ip": "10.0.0.x",
      "depends_on": ["other_container"],
      "restart": "always|on-failure|no"
    }
  },
  "networks": {
    "network_name": {
      "driver": "bridge|host|none",
      "subnet": "10.0.0.0/24",
      "gateway": "10.0.0.1"
    }
  }
}

Common images and their defaults:
- nginx: nginx:latest, port 80, common for web servers
- redis: redis:alpine, port 6379, key-value store
- postgres: postgres:latest, port 5432, needs POSTGRES_PASSWORD, POSTGRES_USER, POSTGRES_DB env vars
- mysql: mysql:latest, port 3306, needs MYSQL_ROOT_PASSWORD, MYSQL_DATABASE env vars
- python: python:3.11-slim
- node: node:18-alpine
- ubuntu: ubuntu:22.04
- alpine: alpine:latest

Rules:
1. If multiple containers are mentioned, create separate entries in "containers"
2. If containers need to communicate, add a bridge network
3. Use sensible defaults (postgres gets default env vars, nginx gets port 80)
4. For restart policies: use "always" for production services, "on-failure" for jobs
5. Dependencies: if one service needs another (web needs database), use depends_on
6. Port mappings: hostPort:containerPort (e.g., "8080:80" maps host 8080 to container 80)

Respond ONLY with valid JSON. No explanations, no markdown, just the JSON config."""

    async def parse(self, prompt: str) -> tuple[Dict, str]:
        """Parse natural language prompt into Kitten config using OpenAI"""

        try:
            # Call OpenAI API
            response = client.chat.completions.create(
                model="gpt-4",
                messages=[
                    {"role": "system", "content": self.system_prompt},
                    {"role": "user", "content": prompt}
                ],
                temperature=0.3,
                max_tokens=2000
            )

            config_text = response.choices[0].message.content.strip()

            # Remove markdown code blocks if present
            if config_text.startswith("```"):
                config_text = config_text.split("```")[1]
                if config_text.startswith("json"):
                    config_text = config_text[4:]
                config_text = config_text.strip()

            # Parse JSON
            config = json.loads(config_text)

            # Generate explanation
            explanation = await self._generate_explanation(prompt, config)

            return config, explanation

        except json.JSONDecodeError as e:
            raise HTTPException(
                status_code=500,
                detail=f"OpenAI returned invalid JSON: {str(e)}"
            )
        except Exception as e:
            raise HTTPException(
                status_code=500,
                detail=f"OpenAI API error: {str(e)}"
            )

    async def _generate_explanation(self, prompt: str, config: Dict) -> str:
        """Generate human-readable explanation of what will be deployed"""

        container_count = len(config.get("containers", {}))
        network_count = len(config.get("networks", {}))

        explanation_parts = []

        # Containers summary
        if container_count > 0:
            container_names = list(config["containers"].keys())
            explanation_parts.append(
                f"**Containers:** {container_count} container(s) - {', '.join(container_names)}")

        # Networks summary
        if network_count > 0:
            explanation_parts.append(
                f"**Networks:** {network_count} network(s) created for inter-container communication")

        # Port mappings
        ports = []
        for name, spec in config.get("containers", {}).items():
            if "ports" in spec:
                for port in spec["ports"]:
                    ports.append(f"{name}: {port}")

        if ports:
            explanation_parts.append(f"**Exposed Ports:** {', '.join(ports)}")

        # Dependencies
        deps = []
        for name, spec in config.get("containers", {}).items():
            if "depends_on" in spec:
                for dep in spec["depends_on"]:
                    deps.append(f"{name} depends on {dep}")

        if deps:
            explanation_parts.append(f"**Dependencies:** {', '.join(deps)}")

        return "\n\n".join(explanation_parts)


# Initialize parser
parser = KittenAIParser()

# Web UI


@app.get("/", response_class=HTMLResponse)
async def web_ui():
    """Interactive web interface"""
    return r"""
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Kitten AI Gateway 🐱</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }
        
        body {
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            min-height: 100vh;
            padding: 20px;
        }
        
        .container {
            max-width: 1200px;
            margin: 0 auto;
        }
        
        .header {
            text-align: center;
            color: white;
            margin-bottom: 40px;
        }
        
        .header h1 {
            font-size: 3em;
            margin-bottom: 10px;
            text-shadow: 2px 2px 4px rgba(0,0,0,0.2);
        }
        
        .header p {
            font-size: 1.2em;
            opacity: 0.9;
        }
        
        .card {
            background: white;
            border-radius: 12px;
            padding: 30px;
            box-shadow: 0 10px 30px rgba(0,0,0,0.2);
            margin-bottom: 20px;
        }
        
        .input-section {
            margin-bottom: 20px;
        }
        
        label {
            display: block;
            font-weight: 600;
            margin-bottom: 10px;
            color: #333;
            font-size: 18px;
        }
        
        textarea {
            width: 100%;
            padding: 15px;
            border: 2px solid #e0e0e0;
            border-radius: 8px;
            font-size: 16px;
            font-family: inherit;
            resize: vertical;
            min-height: 120px;
            transition: border-color 0.3s;
        }
        
        textarea:focus {
            outline: none;
            border-color: #667eea;
        }
        
        .button {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            border: none;
            padding: 15px 40px;
            font-size: 16px;
            font-weight: 600;
            border-radius: 8px;
            cursor: pointer;
            transition: transform 0.2s, box-shadow 0.2s;
        }
        
        .button:hover:not(:disabled) {
            transform: translateY(-2px);
            box-shadow: 0 5px 15px rgba(102, 126, 234, 0.4);
        }
        
        .button:active {
            transform: translateY(0);
        }
        
        .button:disabled {
            background: #ccc;
            cursor: not-allowed;
            transform: none;
        }
        
        .examples {
            background: #f8f9fa;
            padding: 20px;
            border-radius: 8px;
            margin-top: 20px;
        }
        
        .examples h3 {
            margin-bottom: 15px;
            color: #667eea;
        }
        
        .examples ul {
            list-style: none;
            padding-left: 0;
        }
        
        .examples li {
            padding: 10px;
            cursor: pointer;
            color: #555;
            transition: all 0.2s;
            border-radius: 4px;
            margin-bottom: 5px;
        }
        
        .examples li:hover {
            background: #e9ecef;
            color: #667eea;
        }
        
        .examples li::before {
            content: "💬 ";
            margin-right: 8px;
        }
        
        #modal {
            display: none;
            position: fixed;
            top: 0;
            left: 0;
            width: 100%;
            height: 100%;
            background: rgba(0,0,0,0.5);
            z-index: 1000;
            overflow-y: auto;
        }
        
        .modal-content {
            background: white;
            max-width: 1000px;
            margin: 50px auto;
            border-radius: 12px;
            padding: 40px;
            box-shadow: 0 20px 60px rgba(0,0,0,0.3);
        }
        
        .modal-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 20px;
            padding-bottom: 20px;
            border-bottom: 2px solid #f0f0f0;
        }
        
        .modal-header h2 {
            color: #333;
            display: flex;
            align-items: center;
            gap: 10px;
        }
        
        .close {
            font-size: 30px;
            cursor: pointer;
            color: #999;
            line-height: 1;
        }
        
        .close:hover {
            color: #333;
        }
        
        .explanation {
            background: #f8f9fa;
            padding: 20px;
            border-radius: 8px;
            margin-bottom: 20px;
            border-left: 4px solid #667eea;
        }
        
        .explanation h3 {
            margin-bottom: 10px;
            color: #667eea;
        }
        
        .json-preview {
            background: #1e1e1e;
            color: #d4d4d4;
            padding: 20px;
            border-radius: 8px;
            overflow-x: auto;
            margin: 20px 0;
            font-family: 'Courier New', monospace;
            font-size: 14px;
            max-height: 500px;
            overflow-y: auto;
        }
        
        .modal-actions {
            display: flex;
            gap: 15px;
            justify-content: flex-end;
            margin-top: 20px;
        }
        
        .button-secondary {
            background: #6c757d;
        }
        
        .button-success {
            background: linear-gradient(135deg, #11998e 0%, #38ef7d 100%);
        }
        
        .status-message {
            padding: 15px;
            border-radius: 8px;
            margin-top: 20px;
            display: none;
            animation: slideIn 0.3s ease-out;
        }
        
        @keyframes slideIn {
            from {
                opacity: 0;
                transform: translateY(-10px);
            }
            to {
                opacity: 1;
                transform: translateY(0);
            }
        }
        
        .status-message.success {
            background: #d4edda;
            color: #155724;
            border: 1px solid #c3e6cb;
        }
        
        .status-message.error {
            background: #f8d7da;
            color: #721c24;
            border: 1px solid #f5c6cb;
        }
        
        .loading {
            display: inline-block;
            width: 20px;
            height: 20px;
            border: 3px solid rgba(255,255,255,.3);
            border-radius: 50%;
            border-top-color: #fff;
            animation: spin 1s ease-in-out infinite;
            margin-left: 10px;
        }
        
        @keyframes spin {
            to { transform: rotate(360deg); }
        }
        
        .deployment-id {
            background: #e7f3ff;
            padding: 10px;
            border-radius: 4px;
            margin-top: 10px;
            font-family: monospace;
            color: #0056b3;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🐱 Kitten AI Gateway</h1>
            <p>Powered by OpenAI - Just describe what you want to deploy!</p>
        </div>
        
        <div class="card">
            <div class="input-section">
                <label for="prompt">What do you want to deploy?</label>
                <textarea 
                    id="prompt" 
                    placeholder="Example: Deploy a web application with nginx, postgres database, and redis cache. They should be able to communicate with each other."
                ></textarea>
            </div>
            
            <button class="button" onclick="parsePrompt()" id="parseBtn">
                🚀 Generate Configuration
            </button>
            
            <div class="examples">
                <h3>✨ Example Prompts (click to use)</h3>
                <ul>
                    <li onclick="useExample(this)">Deploy nginx web server on port 8080</li>
                    <li onclick="useExample(this)">Run 3 nginx containers behind a load balancer with a shared network</li>
                    <li onclick="useExample(this)">Set up a postgres database and redis cache that can talk to each other</li>
                    <li onclick="useExample(this)">Create a microservices setup with nginx, 2 python API servers, postgres database, and redis, all networked together</li>
                    <li onclick="useExample(this)">Deploy mysql database that always restarts on failure</li>
                    <li onclick="useExample(this)">Run a development stack with node.js app that depends on postgres and redis</li>
                </ul>
            </div>
            
            <div id="statusMessage" class="status-message"></div>
        </div>
    </div>
    
    <div id="modal">
        <div class="modal-content">
            <div class="modal-header">
                <h2>📋 Review Configuration</h2>
                <span class="close" onclick="closeModal()">&times;</span>
            </div>
            
            <div class="explanation" id="explanation"></div>
            
            <h3 style="margin-bottom: 10px;">Generated JSON Configuration:</h3>
            <pre class="json-preview" id="jsonPreview"></pre>
            
            <div class="modal-actions">
                <button class="button button-secondary" onclick="closeModal()">Cancel</button>
                <button class="button button-success" onclick="confirmDeploy()" id="deployBtn">
                    ✅ Deploy Now
                </button>
            </div>
        </div>
    </div>
    
    <script>
        let currentSessionId = null;
        let currentConfig = null;
        
        function useExample(element) {
            document.getElementById('prompt').value = element.textContent;
        }
        
        async function parsePrompt() {
            const prompt = document.getElementById('prompt').value.trim();
            
            if (!prompt) {
                showStatus('Please enter a prompt', 'error');
                return;
            }
            
            const parseBtn = document.getElementById('parseBtn');
            parseBtn.disabled = true;
            parseBtn.innerHTML = '🤖 AI is thinking<span class="loading"></span>';
            
            hideStatus();
            
            try {
                const response = await fetch('/parse', {
                    method: 'POST',
                    headers: {'Content-Type': 'application/json'},
                    body: JSON.stringify({ prompt })
                });
                
                if (!response.ok) {
                    const error = await response.json();
                    throw new Error(error.detail || 'Failed to parse prompt');
                }
                
                const data = await response.json();
                currentSessionId = data.session_id;
                currentConfig = data.config;
                
                // Show modal with config
                document.getElementById('explanation').innerHTML = `
                    <h3>What will be deployed:</h3>
                    <p>${data.explanation.replace(/\n\n/g, '<br><br>').replace(/\*\*/g, '<strong>').replace(/\*\*/g, '</strong>')}</p>
                `;
                document.getElementById('jsonPreview').textContent = JSON.stringify(data.config, null, 2);
                document.getElementById('modal').style.display = 'block';
                
            } catch (error) {
                showStatus(`Error: ${error.message}`, 'error');
            } finally {
                parseBtn.disabled = false;
                parseBtn.innerHTML = '🚀 Generate Configuration';
            }
        }
        
        function closeModal() {
            document.getElementById('modal').style.display = 'none';
        }
        
        async function confirmDeploy() {
            if (!currentSessionId) return;
            
            const deployBtn = document.getElementById('deployBtn');
            deployBtn.disabled = true;
            deployBtn.innerHTML = '🚀 Deploying<span class="loading"></span>';
            
            try {
                const response = await fetch('/confirm', {
                    method: 'POST',
                    headers: {'Content-Type': 'application/json'},
                    body: JSON.stringify({ session_id: currentSessionId })
                });
                
                if (!response.ok) {
                    const error = await response.json();
                    throw new Error(error.detail || 'Deployment failed');
                }
                
                const data = await response.json();
                
                closeModal();
                showStatus(
                    `✅ Deployment started successfully!<br>
                    <div class="deployment-id">Deployment ID: ${data.id}</div>
                    <p style="margin-top: 10px;">Containers are being spawned in the background.</p>`, 
                    'success'
                );
                
                // Clear prompt
                document.getElementById('prompt').value = '';
                
            } catch (error) {
                showStatus(`Deployment failed: ${error.message}`, 'error');
            } finally {
                deployBtn.disabled = false;
                deployBtn.innerHTML = '✅ Deploy Now';
            }
        }
        
        function showStatus(message, type) {
            const status = document.getElementById('statusMessage');
            status.innerHTML = message;
            status.className = `status-message ${type}`;
            status.style.display = 'block';
        }
        
        function hideStatus() {
            document.getElementById('statusMessage').style.display = 'none';
        }
        
        // Close modal on escape key
        document.addEventListener('keydown', (e) => {
            if (e.key === 'Escape') closeModal();
        });
        
        // Close modal on outside click
        document.getElementById('modal').addEventListener('click', (e) => {
            if (e.target.id === 'modal') closeModal();
        });
    </script>
</body>
</html>
"""

# API Endpoints


@app.post("/parse", response_model=ParseResponse)
async def parse_prompt(request: NLPRequest):
    """
    Parse natural language into configuration using OpenAI
    Returns session ID and config for review
    """
    try:
        # Parse using OpenAI
        config, explanation = await parser.parse(request.prompt)

        if not config.get("containers"):
            raise HTTPException(
                status_code=400,
                detail="Could not identify any containers to deploy from your prompt"
            )

        # Generate session ID
        session_id = str(uuid.uuid4())

        # Store in pending
        pending_deployments[session_id] = {
            "prompt": request.prompt,
            "config": config,
            "timestamp": datetime.now().isoformat()
        }

        return ParseResponse(
            session_id=session_id,
            prompt=request.prompt,
            config=config,
            explanation=explanation
        )

    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@app.post("/confirm", response_model=DeploymentResponse)
async def confirm_deployment(request: ConfirmRequest):
    """
    Confirm and deploy the configuration after user review
    """
    # Get pending deployment
    pending = pending_deployments.get(request.session_id)

    if not pending:
        raise HTTPException(
            status_code=404, detail="Session not found or expired")

    config = pending["config"]

    try:
        # Forward to Kitten API
        async with httpx.AsyncClient(timeout=30.0) as http_client:
            response = await http_client.post(
                f"{KITTEN_API_URL}/spawn",
                json=config
            )
            response.raise_for_status()
            result = response.json()

        # Cleanup pending
        del pending_deployments[request.session_id]

        return DeploymentResponse(
            id=result["id"],
            message=result["message"],
            status=result["status"],
            config=config
        )

    except httpx.HTTPError as e:
        raise HTTPException(
            status_code=503, detail=f"Kitten API error: {str(e)}")


@app.get("/status/{deployment_id}")
async def get_status(deployment_id: str):
    """Get deployment status from Kitten API"""
    try:
        async with httpx.AsyncClient() as http_client:
            response = await http_client.get(f"{KITTEN_API_URL}/status/{deployment_id}")
            response.raise_for_status()
            return response.json()
    except httpx.HTTPError as e:
        raise HTTPException(
            status_code=503, detail=f"Kitten API error: {str(e)}")


@app.post("/stop/{deployment_id}")
async def stop_deployment(deployment_id: str):
    """Stop a deployment"""
    try:
        async with httpx.AsyncClient() as http_client:
            response = await http_client.post(f"{KITTEN_API_URL}/stop/{deployment_id}")
            response.raise_for_status()
            return response.json()
    except httpx.HTTPError as e:
        raise HTTPException(
            status_code=503, detail=f"Kitten API error: {str(e)}")


@app.get("/list")
async def list_deployments():
    """List all deployments"""
    try:
        async with httpx.AsyncClient() as http_client:
            response = await http_client.get(f"{KITTEN_API_URL}/list")
            response.raise_for_status()
            return response.json()
    except httpx.HTTPError as e:
        raise HTTPException(
            status_code=503, detail=f"Kitten API error: {str(e)}")


@app.get("/health")
async def health_check():
    """Health check"""
    health = {
        "nlp_gateway": "healthy",
        "openai": "configured" if OPENAI_API_KEY else "missing_api_key",
        "kitten_api": "unknown",
        "timestamp": datetime.now().isoformat()
    }

    try:
        async with httpx.AsyncClient(timeout=5.0) as http_client:
            response = await http_client.get(f"{KITTEN_API_URL}/health")
            if response.status_code == 200:
                health["kitten_api"] = "healthy"
            else:
                health["kitten_api"] = "unhealthy"
    except:
        health["kitten_api"] = "unreachable"

    return health

if __name__ == "__main__":
    import uvicorn
    print("🐱 Starting Kitten AI Gateway (OpenAI-powered)...")
    print(f"OpenAI API Key: {
          '✅ Configured' if OPENAI_API_KEY else '❌ Missing'}")
    print("\nOpen in browser: http://localhost:8000")
    uvicorn.run(app, host="0.0.0.0", port=8000)

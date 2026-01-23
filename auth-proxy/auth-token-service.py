import argparse
import httpx
import secrets
import os
import uvicorn
from fastapi import FastAPI, Request, Response
from urllib.parse import urlparse, parse_qs

app = FastAPI()

# Configuration constants
TOKEN_FILE = ".last_token"
NTFY_URL = "https://ntfy.sh/mytv-X7kgmipX"

def get_stored_token():
    if os.path.exists(TOKEN_FILE):
        with open(TOKEN_FILE, "r") as f:
            return f.read().strip()
    return None

def save_token(token):
    with open(TOKEN_FILE, "w") as f:
        f.write(token)

@app.get("/validate")
async def validate_request(request: Request):
    # Use the domain passed via command line arguments
    domain = app.state.domain
    
    original_uri = request.headers.get("X-Original-URI", "/")
    parsed_uri = urlparse(original_uri)
    query_params = parse_qs(parsed_uri.query)
    
    received_token = query_params.get("secure", [None])[0]
    current_valid_token = get_stored_token()

    if not received_token:
        new_token = secrets.token_hex(16)
        save_token(new_token)
        
        # Build the URL using the explicit domain argument
        secure_url = "https://{domain}{path}?secure={token}".format(domain=domain, path=parsed_uri.path, token=new_token)
        
        async with httpx.AsyncClient() as client:
            await client.post(
                NTFY_URL,
                content="New access attempt. Authorized URL: {secure_url}".format(secure_url=secure_url),
                headers={"Title": "Security Alert - MyTV"}
            )
        
        return Response(status_code=403, content="Access Denied: Token sent via ntfy.")

    if received_token == current_valid_token:
        return Response(status_code=200)

    return Response(status_code=403, content="Access Denied: Invalid token.")

if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Auth Proxy Middleware")
    parser.add_argument("--port", type=int, default=8000)
    parser.add_argument("--host", type=str, default="127.0.0.1")
    parser.add_argument("--domain", type=str, required=True, help="Your public domain (e.g. mytv.com)")

    args = parser.parse_args()

    # Store the domain in app state so it's accessible in the routes
    app.state.domain = args.domain

    uvicorn.run(app, host=args.host, port=args.port)
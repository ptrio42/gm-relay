# GM Relay

A Nostr relay that only accepts GM notes once a day.

# Live instance

- wss://gm.swarmstr.com

Simply add it to your relay list & forget it.
It will collect all your GMs daily.

# Running your own instance

If you'd like to run your own instance or modify relays behavior (eg. accept different kind of notes), here's how to do it:

## Prerequisites

- **Go**: Ensure you have Go installed on your system. You can download it from [here](https://golang.org/dl/).

## Setup

### 1. Clone the repository

```bash
git clone https://github.com/ptrio42/gm-relay.git
cd gm-relay
```

### 2. Start the Project with Docker Compose

 ```sh
 # in foreground
 docker compose up --build
 # in background
 docker compose up --build -d
 ```

### 3. Access the relay

```bash
http://localhost:3336
```

## Serving over nginx (optional)

```nginx
server {
    server_name gm.swarmstr.com;

    location / {
        proxy_pass http://localhost:3336;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
```

## Install certificate

```bash
sudo certbot --nginx -d gm.swarmstr.com
```

## License

This project is licensed under the MIT License.

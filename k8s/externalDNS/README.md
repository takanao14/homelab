# ExternalDNS for PowerDNS (Home Lab)

This repository manages the ExternalDNS configuration used to automatically register DNS records to PowerDNS (pdns) from a home lab Kubernetes cluster.

It utilizes Kustomize to manage different configurations for development (`dev`) and production (`prd`) environments.

## Directory Structure

```text
.
├── README.md           # This file
├── namespace.yaml      # Common Namespace definition
├── base/               # Common base resources
│   ├── deployment.yaml # ExternalDNS Deployment
│   ├── kustomization.yaml
│   └── rbac.yaml       # ServiceAccount and RBAC settings
└── overlays/           # Environment-specific configurations
    ├── dev/
    │   ├── .env        # Secret variables for development (not committed)
    │   ├── .env.sample # Template for .env (committed)
    │   └── kustomization.yaml
    └── prd/
        ├── .env        # Secret variables for production (not committed)
        ├── .env.sample # Template for .env (committed)
        └── kustomization.yaml
```

## Prerequisites

- A PowerDNS server must be running with the HTTP API enabled.
- Network connectivity from the Kubernetes cluster to the PowerDNS API must be established.

## Deployment

### 1. Create Namespace
```bash
kubectl apply -f namespace.yaml
```

### 2. Create `.env` from Template
Copy the provided sample file and fill in the actual values for your environment:

```bash
# For development
cp overlays/dev/.env.sample overlays/dev/.env
# Edit overlays/dev/.env with actual values

# For production
cp overlays/prd/.env.sample overlays/prd/.env
# Edit overlays/prd/.env with actual values
```

### 3. Environment-Specific Deployment
After verifying and editing the contents of `overlays/<env>/.env`, run the following commands:

**Development (dev):**
```bash
kubectl apply -k overlays/dev
```

**Production (prd):**
```bash
kubectl apply -k overlays/prd
```

## Configuration (.env)

Define the following variables in the `.env` file for each environment. These are expanded as a Kubernetes Secret (`external-dns-env`) via `secretGenerator`.

| Variable Name | Description | Example |
| :--- | :--- | :--- |
| `PDNS_API_URL` | PowerDNS API endpoint | `http://192.168.xx.yy:8081` |
| `PDNS_SERVER_ID` | PowerDNS server ID | `localhost` |
| `PDNS_HTTP_API_KEY` | PowerDNS API key | `your-secret-key` |
| `OWNER_ID_FOR_THIS_EXTERNAL_DNS` | ID to identify records managed by this ExternalDNS instance | `externaldns-cluster` |
| `DOMAIN_FILTER` | Filter for the target domains to be processed | `dev.example.com.` |

## Notes

- **Security:** The `.env` files contain sensitive information such as API keys. Handle them with care.
  - `.env` files are listed in `.gitignore` and **must not be committed** to the repository.
  - `.env.sample` files serve as commit-safe templates. Keep them up to date whenever new variables are added.
- **Resource Conflicts:** If you are deploying multiple environments to the same Kubernetes cluster, you may need to specify a `namePrefix` in `kustomization.yaml` to avoid naming collisions.

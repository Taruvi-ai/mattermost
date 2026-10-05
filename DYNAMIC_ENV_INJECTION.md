# Dynamic Environment Variable Injection for Kubernetes Operator-Managed Deployments

## Problem Statement

Environment variables from Infisical were not being injected into pods managed by the Mattermost Kubernetes Operator. The Operator's CRD limitations and database-first config approach prevented standard env var injection methods.

## Root Cause

1. **Database Config Override**: The Mattermost Operator automatically sets `MM_CONFIG` to point to the database (from `database.external.secret`), which forces Mattermost to read ALL configuration from the database instead of environment variables.
2. **Operator Limitations**: The Mattermost Operator CRD doesn't support `envFrom` field, only individual `mattermostEnv` entries.
3. **Dynamic Deployment**: The Operator generates the deployment dynamically from the CRD, so there's no static deployment file to edit.

## Solutions Attempted (and Why They Failed)

### ❌ Attempt 1: Bake envs into Docker image
- **Approach**: Use Docker build args to bake Infisical vars into the image
- **Why it failed**: Secrets in image layers are a security risk, and requires rebuild for every env change

### ❌ Attempt 2: Add envs to CRD mattermostEnv
- **Approach**: Manually list each env var in the CRD with `valueFrom.secretKeyRef`
- **Why it failed**: Requires manual updates to CRD for every new Infisical variable (not scalable)

### ❌ Attempt 3: Use CRD mattermostEnvFrom
- **Approach**: Reference secret using `mattermostEnvFrom` in CRD
- **Why it failed**: The Mattermost Operator CRD doesn't support `mattermostEnvFrom` field

## Final Solution ✅

### Architecture
1. **Disable DB Config**: Set `MM_CONFIG=""` in CRD to force Mattermost to use environment variables instead of database config
2. **Dynamic Secret Creation**: GitHub Actions workflow creates/updates K8s secret from ALL Infisical vars
3. **Deployment Patch**: Workflow patches the Operator-generated deployment to add `envFrom` pointing to the secret
4. **Automatic Restart**: Workflow restarts deployment to apply changes

### Implementation

#### 1. CRD Configuration (`self-mattermost/mattermost-installation.yaml`)
```yaml
mattermostEnv:
  - name: MM_CONFIG
    value: ""  # Forces env var usage instead of DB config
  - name: MM_FILESETTINGS_AMAZONS3SSE
    value: "true"
  # ... other static envs
```

#### 2. GitHub Actions Workflow (`.github/workflows/harbor-push.yml`)

**Build Job:**
- Fetches Infisical vars (for Harbor credentials)
- Builds and pushes Docker image
- Uses GitHub Actions cache for faster builds

**Deploy Job:**
- Fetches Infisical vars
- Creates K8s secret `mm-infisical` from ALL `MM_*` environment variables
- Patches deployment to add `envFrom` referencing the secret
- Restarts deployment

```yaml
- name: Create/Update Infisical Secret
  run: |
    printenv | grep '^MM_' > /tmp/mm.env
    kubectl create secret generic mm-infisical --from-env-file=/tmp/mm.env -n mattermost --dry-run=client -o yaml | kubectl apply -f -

- name: Patch deployment to use envFrom
  run: |
    kubectl patch deployment mattermost-staging -n mattermost --type=strategic -p='{"spec":{"template":{"spec":{"containers":[{"name":"mattermost","envFrom":[{"secretRef":{"name":"mm-infisical"}}]}]}}}}'
```

### Why This Works

1. **MM_CONFIG=""** tells Mattermost to ignore database config and use environment variables
2. **K8s Secret** stores all Infisical vars dynamically (no hardcoding)
3. **envFrom** loads ALL vars from secret into pod environment
4. **Deployment Patch** persists on every workflow run, overriding Operator's generated deployment
5. **Scalable**: Adding new vars to Infisical automatically includes them (no code changes needed)

## Key Learnings

1. **Mattermost Operator Behavior**: The Operator auto-sets `MM_CONFIG` when `database.external.secret` is configured, which overrides env vars
2. **CRD Limitations**: Not all Kubernetes features are exposed in CRDs (e.g., `envFrom`)
3. **Operator-Generated Resources**: Can't edit deployment files directly; must patch them programmatically
4. **Distroless Images**: The original Dockerfile used distroless base which prevented shell access; switched to Ubuntu for debugging

## Files Modified

1. `mattermost/.github/workflows/harbor-push.yml` - Added deploy job with secret creation and deployment patch
2. `EOS-kubernetes/self-mattermost/mattermost-installation.yaml` - Added `MM_CONFIG=""` to force env var usage
3. `mattermost/Dockerfile` - Changed from distroless to Ubuntu base for shell access (debugging)

## Adding New Environment Variables

1. Add variable to Infisical under `/Mattermost` path in `taruvi-test` environment
2. Push code to `mattermost-taruvi` branch
3. Workflow automatically creates/updates secret and restarts pods
4. No code changes required

## Verification

Check pod environment variables:
```bash
kubectl exec -it pod/<pod-name> -n mattermost -- env | grep MM_TARUVI
```

Check logs for correct URL:
```bash
kubectl logs pod/<pod-name> -n mattermost | grep "Taruvi authentication request"
```

Expected: `"url":"https://test-api.taruvi.cloud/..."`

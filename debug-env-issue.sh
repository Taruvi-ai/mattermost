#!/bin/bash
# Debug script to check why env vars reverted to localhost

echo "=== Checking mm-infisical secret ==="
kubectl get secret mm-infisical -n mattermost -o jsonpath='{.data.MM_TARUVI_SERVER_URL}' | base64 -d
echo -e "\n"

echo "=== Checking deployment envFrom ==="
kubectl get deployment mattermost-staging -n mattermost -o jsonpath='{.spec.template.spec.containers[0].envFrom}'
echo -e "\n"

echo "=== Checking pod environment ==="
POD=$(kubectl get pods -n mattermost -l app=mattermost-staging -o jsonpath='{.items[0].metadata.name}')
kubectl exec -n mattermost $POD -- env | grep MM_TARUVI_SERVER_URL

echo -e "\n=== Checking CRD mattermostEnv ==="
kubectl get mattermost mattermost-staging -n mattermost -o jsonpath='{.spec.mattermostEnv}' | jq

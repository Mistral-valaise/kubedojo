set -e
mkdir -p /opt/course/jackal/overlays/staging
cat > /opt/course/jackal/overlays/staging/kustomization.yaml <<'YAML'
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - ../../base
namespace: app-jackal
namePrefix: stg-
labels:
  - pairs:
      stage: staging
images:
  - name: nginx
    newTag: 1.27-alpine
replicas:
  - name: reporter
    count: 2
configMapGenerator:
  - name: reporter-config
    literals:
      - REPORT_INTERVAL=30s
YAML
kubectl apply -k /opt/course/jackal/overlays/staging
kubectl -n app-jackal rollout status deployment/stg-reporter --timeout=180s

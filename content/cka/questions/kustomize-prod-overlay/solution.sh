set -e
mkdir -p /opt/course/kustomize/overlays/prod
cat > /opt/course/kustomize/overlays/prod/kustomization.yaml <<'YAML'
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - ../../base
namespace: team-swift
namePrefix: prod-
replicas:
  - name: notifier
    count: 3
labels:
  - pairs:
      env: prod
YAML
kubectl apply -k /opt/course/kustomize/overlays/prod

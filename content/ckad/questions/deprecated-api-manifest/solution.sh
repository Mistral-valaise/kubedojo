set -e
cd /opt/course/marmot
sed -i 's|^apiVersion: extensions/v1beta1|apiVersion: apps/v1|' deployment.yaml
sed -i 's|^  replicas: 2|  replicas: 2\n  selector:\n    matchLabels:\n      app: reports|' deployment.yaml
sed -i 's|^apiVersion: batch/v1beta1|apiVersion: batch/v1|' cronjob.yaml
sed -i -e 's|^apiVersion: autoscaling/v2beta1|apiVersion: autoscaling/v2|' \
       -e 's|^    apiVersion: extensions/v1beta1|    apiVersion: apps/v1|' \
       -e 's|^        targetAverageUtilization: 70|        target:\n          type: Utilization\n          averageUtilization: 70|' hpa.yaml
kubectl apply -f deployment.yaml -f cronjob.yaml -f hpa.yaml
kubectl -n app-marmot rollout status deployment reports --timeout=180s
kubectl api-resources --no-headers | awk '$NF == "PodDisruptionBudget" {print $(NF-2)}' > pdb-version
[ -s pdb-version ] || { echo "kubectl api-resources lists no PodDisruptionBudget" >&2; exit 1; }

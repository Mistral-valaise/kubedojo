set -e
kubectl -n team-orca create serviceaccount pipeline
kubectl -n team-orca create role pipeline --verb=create,list --resource=deployments.apps,configmaps
kubectl -n team-orca create rolebinding pipeline --role=pipeline --serviceaccount=team-orca:pipeline

set -e
kubectl -n app-seal create serviceaccount job-watcher
kubectl -n app-seal create role job-reader --verb=get,list,watch --resource=jobs.batch
kubectl -n app-seal create rolebinding job-watcher-reads-jobs --role=job-reader --serviceaccount=app-seal:job-watcher
kubectl -n app-seal set serviceaccount deployment job-watcher job-watcher
kubectl -n app-seal rollout status deployment job-watcher --timeout=120s

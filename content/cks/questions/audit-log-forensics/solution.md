```bash
ssh {{host}}
cd /opt/course/forensics
wc -l audit.log
jq -r '.verb' audit.log | sort | uniq -c
```

**1 + 2. Who deleted the Secret, and from where?**

```bash
jq -c 'select(.objectRef.resource=="secrets" and .objectRef.name=="db-credentials" and .verb=="delete" and .stage=="ResponseComplete")
       | [.requestReceivedTimestamp, .user.username, .sourceIPs[0], .objectRef.namespace, .responseStatus.code]' audit.log
# ["2026-03-14T01:31:40.550908Z","dana","192.168.40.17","payments",403]
# ["2026-03-14T01:47:03.664871Z","system:serviceaccount:ci:deploy-bot","10.244.2.91","payments",200]
# ["2026-03-14T02:05:10.220018Z","kubernetes-admin","172.18.0.1","staging",200]
```

Three delete requests for a Secret of that name. `dana` was denied (403). `kubernetes-admin` deleted a
Secret of the same name in another Namespace. The one that matters:

```bash
echo 'system:serviceaccount:ci:deploy-bot' > user
echo '10.244.2.91' > source-ip
```

**3. What did that identity do before?**

```bash
jq -c 'select(.user.username=="system:serviceaccount:ci:deploy-bot" and .stage=="ResponseComplete")
       | [.requestReceivedTimestamp, .sourceIPs[0], .verb, .objectRef.resource, .objectRef.name, .responseStatus.code]' audit.log | tail -8
# [...,"10.244.2.91","list","secrets",null,403]
# [...,"10.244.2.91","create","clusterrolebindings","ci-maintenance",201]
# [...,"10.244.2.91","list","secrets",null,200]
# [...,"10.244.2.91","get","secrets","db-credentials",200]
# [...,"10.244.2.91","delete","secrets","db-credentials",200]
echo 'ci-maintenance' > binding
```

**Reading it as an attack:** the ServiceAccount normally talks from `10.244.1.30` and lists Deployments.
At 01:38 its token shows up from another address (`10.244.2.91`), is denied on Secrets, creates a
ClusterRoleBinding for itself at 01:41 — it was allowed to create bindings, which is the real
misconfiguration — and three minutes later reads and deletes the Secret. Next steps in real life:
delete the binding, rotate the token and the database credentials, and find the Pod at `10.244.2.91`.

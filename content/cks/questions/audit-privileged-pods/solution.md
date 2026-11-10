```bash
ssh {{host}}
k -n sec-wader get pod
k -n sec-wader get pod -o json | jq -r '
  .items[]
  | select(
      .spec.hostNetwork
      or .spec.hostPID
      or any(.spec.volumes[]?; .hostPath)
      or any(.spec.containers[]; .securityContext.privileged)
    )
  | .metadata.name' | tee /opt/course/audit/dangerous-pods
# debug-shell
# log-shipper
# net-tools
```

Without jq, a table works too:

```bash
k -n sec-wader get pod -o custom-columns='NAME:.metadata.name,HOSTNET:.spec.hostNetwork,HOSTPID:.spec.hostPID,HOSTPATH:.spec.volumes[*].hostPath.path,PRIV:.spec.containers[*].securityContext.privileged'
# NAME           HOSTNET   HOSTPID   HOSTPATH   PRIV
# api            <none>    <none>    <none>     false
# cache-warmer   <none>    <none>    <none>     <none>
# debug-shell    <none>    <none>    <none>     true
# log-shipper    <none>    <none>    /var/log   <none>
# net-tools      true      <none>    <none>     <none>
```

Watch the details: `debug-shell` is privileged only in its **second** container, and `api` mentions
`privileged` but sets it to `false`.

```bash
k -n sec-wader delete pod debug-shell log-shipper net-tools
```

To keep such Pods out permanently, enforce the `baseline` Pod Security Standard on the Namespace — it
forbids all three.

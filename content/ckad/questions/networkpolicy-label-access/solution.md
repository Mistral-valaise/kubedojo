The policies are fine and must stay as they are, so read them and find out which labels they are
waiting for:

```bash
ssh {{host}}
k -n app-tiger exec deploy/checkout -- wget -qO- -T 3 http://ledger      # download timed out
k -n app-tiger get netpol
#   NAME                   POD-SELECTOR
#   allow-dns              <none>
#   default-deny           <none>
#   ledger-client-egress   role=ledger-client
#   ledger-ingress         app=ledger
k -n app-tiger describe netpol ledger-ingress ledger-client-egress
k -n app-tiger get pod --show-labels
```

- `default-deny` selects every Pod and allows nothing: all traffic in **both** directions is blocked
  unless another policy allows it. `allow-dns` re-opens port 53 for everyone.
- `ledger-ingress` protects `app=ledger` and admits Pods labeled `ledger-access=granted` on port 80.
- `ledger-client-egress` lets Pods labeled `role=ledger-client` send to `app=ledger` on port 80.

A connection needs both halves: the client must be allowed to send (egress) **and** the ledger must be
allowed to receive from it (ingress). So the checkout Pods need two labels, and no Pod has them yet.
The Pods belong to a Deployment, therefore the labels go into the Pod template; `k label pod` would be
lost with the next rollout.

```bash
k -n app-tiger patch deploy checkout --type=merge \
  -p '{"spec":{"template":{"metadata":{"labels":{"role":"ledger-client","ledger-access":"granted"}}}}}'
k -n app-tiger rollout status deploy checkout
```

or `k -n app-tiger edit deploy checkout`:

```yaml
spec:
  template:
    metadata:
      labels:
        app: checkout               # keep: the Deployment's selector needs it
        role: ledger-client
        ledger-access: granted
```

Verify both sides of the requirement:

```bash
k -n app-tiger get pod -l role=ledger-client,ledger-access=granted          # the new checkout Pod
k -n app-tiger exec deploy/checkout -- wget -qO- -T 3 http://ledger | head -4   # Welcome to nginx
k -n app-tiger exec deploy/reports -- wget -qO- -T 3 http://ledger              # still times out
```

**Takeaway:** in the exam the policies are usually given and correct; your job is to make a Pod match
them. Read `podSelector` (who the policy applies to) and `from`/`to` (who the peer is), and remember
that with a default deny in both directions a client needs an egress allowance as well as the
server's ingress allowance. `k label deploy checkout ...` labels the Deployment object, not its Pods.

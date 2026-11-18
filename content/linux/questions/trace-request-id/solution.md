Start from what the customer knows (the order number), turn it into what the systems know (the
request id), then follow that id through every log.

**1. From the order to the request id.**

```bash
ssh {{host}}
cd /opt/course/trace-request-id
head -n 3 logs/*.log                       # three formats: plain, JSON, bracketed
grep ORD-7731 logs/gateway.log
# 10:15:01.204Z gateway req=7f3a9c POST /orders order=ORD-7731 ...
# 10:16:40.087Z gateway req=b81d02 POST /orders order=ORD-7731 ...
# 10:16:52.310Z gateway req=...    GET /orders/ORD-7731 ...
grep -E 'req=(7f3a9c|b81d02) status' logs/gateway.log
# req=7f3a9c status=502 ...      <- the failed attempt
# req=b81d02 status=201 ...
echo 7f3a9c > request-id.txt
```

**2. Collect the trace.**

```bash
grep -hw 7f3a9c logs/*.log > trace.txt
cat trace.txt                              # 8 lines
```

- `-h` leaves out the `logs/gateway.log:` prefix that grep adds when it searches several files.
- `-w` matches the id as a whole word only. Without it, a ninth line slips in: a payment of
  another request whose transaction id is `a07f3a9c`. Compare `grep -c 7f3a9c logs/payments.log`
  with `grep -cw 7f3a9c logs/payments.log`.

For the JSON log, `jq` selects by field instead of by text, which cannot match by accident:

```bash
jq -c 'select(.request_id == "7f3a9c")' logs/orders.log
```

(`jq -c` prints compact JSON, here identical to the original lines.)

**3. Read the trace along the call chain.**

```text
gateway   req=7f3a9c status=502 upstream=orders        "my upstream failed"
orders    "msg":"payments returned 504"                "my upstream failed"
payments  ERROR rid=7f3a9c card-processor timeout after 3000ms, giving up
```

Gateway and orders only pass on what they received. `payments` is the deepest service with an error
of its own: it waited 3 seconds for the card processor and gave up.

```bash
echo payments > failing-service.txt
```

**Takeaway:** find the id, grep it as a whole word across all logs, and follow the error down the
call chain to the service that does not blame anybody else.

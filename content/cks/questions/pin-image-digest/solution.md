```bash
ssh {{host}}
k -n sec-swift get pod -o jsonpath='{.items[0].status.containerStatuses[0].imageID}'; echo
# docker.io/library/nginx@sha256:65645c7bb6a0661892a8b03b89d0743208a18dd2f3f17a54ef4b76fb8e2f2a10
```

`imageID` is what the container runtime actually resolved the tag to on that node.

```bash
echo sha256:65645c7b... > /opt/course/digest/edge
k -n sec-swift set image deploy edge nginx=docker.io/library/nginx@sha256:65645c7b...
k -n sec-swift rollout status deploy edge
k -n sec-swift get deploy edge -o jsonpath='{.spec.template.spec.containers[0].image}'; echo
```

(Use the digest **your** cluster shows.) On the node, `crictl images --digests` lists the same value.

**Forms of an image reference:**

| Reference | Meaning |
|---|---|
| `nginx` | `docker.io/library/nginx:latest` — avoid |
| `nginx:1.27-alpine` | a tag; can be re-pushed with different content |
| `nginx@sha256:…` | exactly one image, forever |
| `nginx:1.27-alpine@sha256:…` | digest wins; the tag is only documentation |

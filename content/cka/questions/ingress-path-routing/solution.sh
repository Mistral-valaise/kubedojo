set -e
kubectl -n team-wren create ingress shop --class=nginx \
  --rule="shop.wren.example/api*=shop-api:8080" --rule="shop.wren.example/*=shop:80"

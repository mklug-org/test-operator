# kubectl delete -f config/samples/webserver* || true && sleep 5 && kubectl apply -f config/samples/webserver* && time ./speed-test.sh

ATTEMPTS=0
ROLLOUT_STATUS_CMD="kubectl get ingress nginx-sample"
until $ROLLOUT_STATUS_CMD || [ $ATTEMPTS -eq 10000 ]; do
  $ROLLOUT_STATUS_CMD
  ATTEMPTS=$((attempts + 1))
done
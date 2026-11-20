# expect: fail source-ip
# Right identity, but the address the ServiceAccount normally uses instead of
# the one the delete request came from.
cd /opt/course/forensics
echo system:serviceaccount:ci:deploy-bot > user
echo 10.244.1.30 > source-ip
echo ci-maintenance > binding

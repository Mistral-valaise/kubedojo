set -e
cd /opt/course/forensics
del=$(jq -c 'select(.stage=="ResponseComplete" and .verb=="delete" and .objectRef.resource=="secrets" and .objectRef.namespace=="payments" and .objectRef.name=="db-credentials" and .responseStatus.code<300)' audit.log)
user=$(echo "$del" | jq -r .user.username)
echo "$user" > user
echo "$del" | jq -r '.sourceIPs[0]' > source-ip
jq -r --arg u "$user" 'select(.stage=="ResponseComplete" and .user.username==$u and .verb=="create" and .objectRef.resource=="clusterrolebindings" and .responseStatus.code<300) | .objectRef.name' audit.log > binding

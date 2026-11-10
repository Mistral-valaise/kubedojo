kubectl delete namespace sec-lark --ignore-not-found --wait >/dev/null
kubectl create namespace sec-lark >/dev/null
ssh ${KD_HOST}-worker 'rm -f /var/lib/kubelet/seccomp/profiles/deny-dangerous.json'
mkdir -p /opt/course/seccomp
cat > /opt/course/seccomp/deny-dangerous.json <<'JSON'
{
  "defaultAction": "SCMP_ACT_ALLOW",
  "architectures": ["SCMP_ARCH_X86_64", "SCMP_ARCH_AARCH64"],
  "syscalls": [
    {
      "names": ["ptrace", "unshare", "mount", "umount2", "reboot", "kexec_load", "init_module", "finit_module", "delete_module"],
      "action": "SCMP_ACT_ERRNO"
    }
  ]
}
JSON
chown -R candidate:candidate /opt/course/seccomp

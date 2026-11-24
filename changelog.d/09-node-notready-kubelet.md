### Fixed

- CKA `node-notready-kubelet`: the "kubelet service is running" check passed at random while the
  kubelet was still broken. systemd reports a unit that it restarts every second as `active` for a
  moment on each attempt; the check now also asks the kubelet's health port. Preparing the question
  fails with the reason when the kubelet does not break as intended, instead of handing out a task
  that is already solved, for example after an earlier attempt that put a kubelet at the wrong path.

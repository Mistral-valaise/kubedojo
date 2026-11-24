// Runs before every test file (see vitest.config.ts): what jsdom lacks and the
// components rely on.

// jsdom has the <dialog> element without its methods. Modal opens with
// showModal(); these stand-ins do what a test can observe of it: the `open`
// attribute, and the `close` event.
if (typeof HTMLDialogElement !== "undefined") {
  const dialog = HTMLDialogElement.prototype;
  if (typeof dialog.showModal !== "function") {
    dialog.showModal = function showModal(this: HTMLDialogElement) { this.setAttribute("open", ""); };
  }
  if (typeof dialog.close !== "function") {
    dialog.close = function close(this: HTMLDialogElement) {
      if (!this.hasAttribute("open")) return;
      this.removeAttribute("open");
      this.dispatchEvent(new Event("close"));
    };
  }
}

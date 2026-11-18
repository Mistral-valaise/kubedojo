import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { choiceView } from "../fixtures";
import { Choice } from "./Choice";

afterEach(cleanup);

/** A Choice that keeps its own value, like a page does. */
function Harness({ multiple = false, onSubmit, hotkeys }: { multiple?: boolean; onSubmit?: () => void; hotkeys?: boolean }) {
  const [value, setValue] = useState<string[]>([]);
  return (
    <>
      <Choice choice={choiceView({ multiple })} value={value} onChange={setValue} onSubmit={onSubmit} hotkeys={hotkeys} />
      <textarea aria-label="Notes" />
      <button>Elsewhere</button>
      <output>{value.join(",")}</output>
    </>
  );
}
const picked = () => screen.getByRole("status").textContent;
const radio = (name: string) => screen.getByRole("radio", { name });
const key = (target: Element, k: string, init: KeyboardEventInit = {}) => fireEvent.keyDown(target, { key: k, ...init });

describe("Choice, single answer", () => {
  it("is a radiogroup whose options are named by their text, with one tab stop", () => {
    render(<Harness />);
    expect(screen.getByRole("radiogroup")).toBeTruthy();
    const options = screen.getAllByRole("radio");
    expect(options.map((o) => o.getAttribute("aria-checked"))).toEqual(["false", "false", "false", "false"]);
    expect(options.map((o) => o.tabIndex)).toEqual([0, -1, -1, -1]);
    // Inline code in an option is text, not a second control inside the radio.
    expect(radio("kubectl get pods").querySelector("code")).toBeTruthy();
    expect(within(radio("kubectl get pods")).queryByRole("button")).toBeNull();
  });

  it("selects by click and replaces the selection", () => {
    render(<Harness />);
    fireEvent.click(radio("kubelet"));
    expect(picked()).toBe("a");
    fireEvent.click(radio("kube-proxy"));
    expect(picked()).toBe("d");
    expect(radio("kube-proxy").getAttribute("aria-checked")).toBe("true");
    expect(radio("kubelet").getAttribute("aria-checked")).toBe("false");
  });

  it("selects the nth option with the keys 1-9 from anywhere on the page", () => {
    render(<Harness />);
    key(document.body, "2");
    expect(picked()).toBe("b");
    expect(document.activeElement).toBe(radio("kube-scheduler"));
    key(screen.getByRole("button", { name: "Elsewhere" }), "4");
    expect(picked()).toBe("d");
    key(document.body, "7"); // there is no seventh option
    expect(picked()).toBe("d");
  });

  it("moves with the arrow keys without selecting, and selects with Space", () => {
    render(<Harness />);
    radio("kubelet").focus();
    key(radio("kubelet"), "ArrowDown");
    expect(document.activeElement).toBe(radio("kube-scheduler"));
    expect(picked()).toBe("");
    key(radio("kube-scheduler"), " ");
    expect(picked()).toBe("b");
    key(radio("kube-scheduler"), "ArrowUp");
    key(radio("kubelet"), "ArrowUp"); // wraps to the last option
    expect(document.activeElement).toBe(radio("kube-proxy"));
    expect(screen.getAllByRole("radio").map((o) => o.tabIndex)).toEqual([-1, -1, -1, 0]);
    expect(picked()).toBe("b");
  });

  it("submits with Enter once something is selected", () => {
    const submit = vi.fn();
    render(<Harness onSubmit={submit} />);
    key(document.body, "Enter");
    expect(submit).not.toHaveBeenCalled();
    key(document.body, "1");
    key(radio("kubelet"), "Enter");
    expect(submit).toHaveBeenCalledTimes(1);
    // Enter on another control belongs to that control.
    key(screen.getByRole("button", { name: "Elsewhere" }), "Enter");
    expect(submit).toHaveBeenCalledTimes(1);
  });

  it("leaves the keys alone while a text field has focus", () => {
    const submit = vi.fn();
    render(<Harness onSubmit={submit} />);
    key(document.body, "1");
    const notes = screen.getByRole("textbox", { name: "Notes" });
    notes.focus();
    expect(key(notes, "3")).toBe(true); // not prevented: the digit is typed
    key(notes, "Enter");
    expect(picked()).toBe("a");
    expect(submit).not.toHaveBeenCalled();
  });

  it("does not take browser shortcuts", () => {
    render(<Harness />);
    expect(key(document.body, "2", { ctrlKey: true })).toBe(true);
    expect(key(document.body, "2", { metaKey: true })).toBe(true);
    expect(key(document.body, "2", { altKey: true })).toBe(true);
    expect(picked()).toBe("");
  });

  it("ignores the keys when the page turns them off", () => {
    render(<Harness hotkeys={false} />);
    key(document.body, "2");
    expect(picked()).toBe("");
  });
});

describe("Choice, several answers", () => {
  it("is a group of checkboxes that toggle by click, number key and Space", () => {
    render(<Harness multiple />);
    expect(screen.getByRole("group")).toBeTruthy();
    expect(screen.getByText("Select every answer that applies.")).toBeTruthy();
    const box = (name: string) => screen.getByRole("checkbox", { name });
    fireEvent.click(box("kubelet"));
    key(document.body, "2");
    expect(picked()).toBe("a,b");
    key(box("kube-scheduler"), " ");
    expect(picked()).toBe("a");
    key(document.body, "1");
    expect(picked()).toBe("");
    expect(box("kubelet").getAttribute("aria-checked")).toBe("false");
  });
});

describe("Choice, review", () => {
  const item = (text: string) => screen.getByText(text).closest("li") as HTMLElement;

  it("marks the correct option and the wrong pick in words, and explains them", () => {
    render(<Choice choice={choiceView({ answer: ["a"] }, true)} value={["a"]} passed={false} />);
    expect(screen.queryByRole("radio")).toBeNull();
    expect(within(screen.getByRole("status")).getByText("Incorrect")).toBeTruthy();
    expect(within(screen.getByRole("list", { name: "Answers" })).getAllByRole("listitem")).toHaveLength(4);
    expect(within(item("kube-scheduler")).getByText("Correct answer")).toBeTruthy();
    expect(within(item("kubelet")).getByText("Your answer · incorrect")).toBeTruthy();
    expect(within(item("kubelet")).getByText("A kubelet starts containers of Pods bound to its node.")).toBeTruthy();
    expect(within(item("kube-proxy")).queryByText(/answer/i)).toBeNull();
    expect(screen.getByText("The scheduler binds Pods to nodes.")).toBeTruthy();
    const ref = screen.getByRole("link", { name: "Kubernetes Scheduler" });
    expect(ref.getAttribute("href")).toBe("https://kubernetes.io/docs/concepts/scheduling-eviction/kube-scheduler/");
    expect(ref.getAttribute("target")).toBe("_blank");
    expect(ref.getAttribute("rel")).toContain("noopener");
  });

  it("confirms a right answer", () => {
    render(<Choice choice={choiceView({ answer: ["b"] }, true)} value={["b"]} passed />);
    expect(within(screen.getByRole("status")).getByText("Correct")).toBeTruthy();
    expect(within(item("kube-scheduler")).getByText("Your answer · correct")).toBeTruthy();
    expect(screen.queryByText("Your answer · incorrect")).toBeNull();
  });

  it("tells an unanswered question from a wrong one, and a revealed one from a graded one", () => {
    const { unmount } = render(<Choice choice={choiceView({}, true)} value={[]} passed={false} />);
    expect(within(screen.getByRole("status")).getByText("Not answered")).toBeTruthy();
    unmount();
    render(<Choice choice={choiceView({}, true)} value={[]} />);
    expect(within(screen.getByRole("status")).getByText("Answer shown")).toBeTruthy();
  });

  it("does not react to keys", () => {
    const change = vi.fn();
    render(<Choice choice={choiceView({ answer: ["a"] }, true)} value={["a"]} onChange={change} passed={false} />);
    key(document.body, "2");
    expect(change).not.toHaveBeenCalled();
  });
});

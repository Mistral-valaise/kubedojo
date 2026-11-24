import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { Field, Input, Select, Switch, useField } from "../ui";

afterEach(cleanup);

const ids = (el: Element) => (el.getAttribute("aria-describedby") ?? "").split(" ").filter(Boolean);
const describedBy = (el: Element) => ids(el).map((id) => document.getElementById(id)?.textContent);

describe("Field with an Input", () => {
  it("ties the label and the description to the control", () => {
    render(<Field label="Base URL" description="Where the model is served."><Input defaultValue="http://localhost" /></Field>);
    const input = screen.getByLabelText("Base URL") as HTMLInputElement;
    expect(input.tagName).toBe("INPUT");
    expect(input.value).toBe("http://localhost");
    expect(describedBy(input)).toEqual(["Where the model is served."]);
    expect(input.getAttribute("aria-invalid")).toBeNull();
  });

  it("marks the control as invalid and announces the error", () => {
    render(<Field label="Model" description="The name the provider knows." error="A model is required."><Input /></Field>);
    const input = screen.getByLabelText("Model");
    expect(input.getAttribute("aria-invalid")).toBe("true");
    expect(describedBy(input)).toEqual(["The name the provider knows.", "A model is required."]);
    expect(screen.getByRole("alert").textContent).toBe("A model is required.");
  });

  it("shows why a value is locked and keeps the control from changing it", () => {
    render(<Field label="Model" locked="Set by --ai-model"><Input value="llama3.2" readOnly /></Field>);
    const input = screen.getByLabelText("Model") as HTMLInputElement;
    expect(input.disabled).toBe(true);
    expect(input.value).toBe("llama3.2");
    expect(screen.getByText("Set by --ai-model")).toBeTruthy();
  });

  it("gives every field its own ids", () => {
    render(<><Field label="One" description="first"><Input /></Field><Field label="Two" description="second"><Input /></Field></>);
    const one = screen.getByLabelText("One"), two = screen.getByLabelText("Two");
    expect(one.id).not.toBe(two.id);
    expect(describedBy(one)).toEqual(["first"]);
    expect(describedBy(two)).toEqual(["second"]);
  });

  it("lets a control of the caller take the same attributes", () => {
    function Custom() {
      const field = useField();
      return <textarea {...field} />;
    }
    render(<Field label="Notes" error="Too long."><Custom /></Field>);
    const area = screen.getByLabelText("Notes");
    expect(area.tagName).toBe("TEXTAREA");
    expect(area.getAttribute("aria-invalid")).toBe("true");
  });
});

describe("Input", () => {
  it("works on its own with a name of its own, and passes what it is given", () => {
    const change = vi.fn();
    render(<Input aria-label="Search" placeholder="Find" mono onChange={change} />);
    const input = screen.getByRole("textbox", { name: "Search" });
    expect(input.getAttribute("placeholder")).toBe("Find");
    expect(input.className.split(" ")).toEqual(expect.arrayContaining(["h-11", "font-mono"]));
    fireEvent.change(input, { target: { value: "pods" } });
    expect(change).toHaveBeenCalledTimes(1);
  });
});

describe("Select", () => {
  it("is the native select, labelled by its field", () => {
    const change = vi.fn();
    render(
      <Field label="Provider">
        <Select defaultValue="openai" onChange={(e) => change(e.target.value)}>
          <option value="anthropic">Anthropic</option><option value="openai">OpenAI-compatible</option>
        </Select>
      </Field>,
    );
    const select = screen.getByRole("combobox", { name: "Provider" }) as HTMLSelectElement;
    expect(select.tagName).toBe("SELECT");
    expect(select.value).toBe("openai");
    fireEvent.change(select, { target: { value: "anthropic" } });
    expect(change).toHaveBeenCalledWith("anthropic");
  });

  it("is disabled in a locked field", () => {
    render(<Field label="Provider" locked="Set by KUBEDOJO_AI_PROVIDER"><Select><option>Anthropic</option></Select></Field>);
    expect((screen.getByRole("combobox", { name: "Provider" }) as HTMLSelectElement).disabled).toBe(true);
  });
});

describe("Switch", () => {
  function Toggle({ disabled }: { disabled?: boolean }) {
    const [on, setOn] = useState(false);
    return <Field layout="row" label="AI assistant" description="Explains failed checks."><Switch checked={on} onChange={setOn} disabled={disabled} /></Field>;
  }

  it("is a switch named by its field that says whether it is on", () => {
    render(<Toggle />);
    const control = screen.getByRole("switch", { name: "AI assistant" });
    expect(control.getAttribute("aria-checked")).toBe("false");
    expect(describedBy(control)).toEqual(["Explains failed checks."]);
    fireEvent.click(control);
    expect(control.getAttribute("aria-checked")).toBe("true");
    // The label belongs to it: a click on the text toggles it too.
    fireEvent.click(screen.getByText("AI assistant"));
    expect(control.getAttribute("aria-checked")).toBe("false");
  });

  it("does not change while it is disabled", () => {
    render(<Toggle disabled />);
    const control = screen.getByRole("switch", { name: "AI assistant" }) as HTMLButtonElement;
    expect(control.disabled).toBe(true);
    fireEvent.click(control);
    expect(control.getAttribute("aria-checked")).toBe("false");
  });

  it("works on its own with a name of its own", () => {
    const change = vi.fn();
    render(<Switch checked aria-label="Traces" onChange={change} />);
    fireEvent.click(screen.getByRole("switch", { name: "Traces" }));
    expect(change).toHaveBeenCalledWith(false);
  });
});

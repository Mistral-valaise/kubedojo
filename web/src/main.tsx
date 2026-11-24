import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router";
import "./styles.css";
import { App } from "./App";

let theme = "dark";
try { theme = localStorage.getItem("kubedojo.theme") || "dark"; } catch { /* ignore */ }
document.documentElement.dataset.theme = theme;

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <BrowserRouter>
      <App />
    </BrowserRouter>
  </StrictMode>,
);

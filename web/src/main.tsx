import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter, Route, Routes } from "react-router";
import "./styles.css";
import { Home } from "./pages/Home";
import { TrackPage } from "./pages/Track";
import { LessonPage } from "./pages/Lesson";
import { PracticePage } from "./pages/Practice";
import { ExamPage } from "./pages/Exam";

let theme = "dark";
try { theme = localStorage.getItem("kubedojo.theme") || "dark"; } catch { /* ignore */ }
document.documentElement.dataset.theme = theme;

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<Home />} />
        <Route path="/t/:track" element={<TrackPage />} />
        <Route path="/t/:track/lesson/:domain" element={<LessonPage />} />
        <Route path="/t/:track/q/:question" element={<PracticePage />} />
        <Route path="/exam/:session" element={<ExamPage />} />
        <Route path="*" element={<Home />} />
      </Routes>
    </BrowserRouter>
  </StrictMode>,
);

import { Route, Routes } from "react-router";
import { ErrorBoundary } from "./components/ui";
import { ExamPage } from "./pages/Exam";
import { Home } from "./pages/Home";
import { LessonPage } from "./pages/Lesson";
import { NotFound } from "./pages/NotFound";
import { PracticePage } from "./pages/Practice";
import { TrackPage } from "./pages/Track";

/**
 * The views of the application by address. An error while rendering one of
 * them is caught here and shown with a way on, instead of an empty page.
 * Rendered inside a router: main.tsx brings the browser's, a test its own.
 */
export function App() {
  return (
    <ErrorBoundary>
      <Routes>
        <Route path="/" element={<Home />} />
        <Route path="/t/:track" element={<TrackPage />} />
        <Route path="/t/:track/lesson/:domain" element={<LessonPage />} />
        <Route path="/t/:track/q/:question" element={<PracticePage />} />
        <Route path="/exam/:session" element={<ExamPage />} />
        <Route path="*" element={<NotFound />} />
      </Routes>
    </ErrorBoundary>
  );
}

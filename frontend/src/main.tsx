import React from "react";
import ReactDOM from "react-dom/client";
import App from "@/App";
import "@/index.css";
import { applyTheme, initialTheme } from "@/hooks/useTheme";

// Apply the saved/OS theme before first paint so there's no light->dark flash.
applyTheme(initialTheme());

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);

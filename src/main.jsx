import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import { installDevSsoBridge } from "./dev/installDevSsoBridge";
import "./index.css";

installDevSsoBridge();

ReactDOM.createRoot(document.getElementById("root")).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);

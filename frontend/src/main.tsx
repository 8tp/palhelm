import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import "./styles/tokens.css";
import "./styles/ui.css";
import "./styles/app.css";
import { AuthProvider } from "./app/AuthProvider";
import { PaletteBridgeProvider } from "./app/paletteBridge";
import { ToastProvider } from "./components/Toast";
import { TooltipProvider } from "./components/Tooltip";
import { CommandPaletteProvider } from "./components/CommandPalette";
import { initTheme } from "./app/theme";
import App from "./app/App";
import { shouldRetryRequest } from "./api/requestPolicy";

initTheme();

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: shouldRetryRequest, refetchOnWindowFocus: false },
  },
});

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <TooltipProvider>
          <ToastProvider>
            <AuthProvider>
              <PaletteBridgeProvider>
                <CommandPaletteProvider>
                  <App />
                </CommandPaletteProvider>
              </PaletteBridgeProvider>
            </AuthProvider>
          </ToastProvider>
        </TooltipProvider>
      </BrowserRouter>
    </QueryClientProvider>
  </StrictMode>,
);

import type { RouteRecord } from "vite-react-ssg";
import { Root } from "./components/Root";
import { Home } from "./pages/Home";
import { Features } from "./pages/Features";
import { Installation } from "./pages/Installation";
import { TmuxSetup } from "./pages/TmuxSetup";
import { Cli } from "./pages/Cli";
import { Configuration } from "./pages/Configuration";
import { TuiPicker } from "./pages/TuiPicker";
import { About } from "./pages/About";

// Routes mirror the previous static pages 1:1 so every canonical URL keeps
// working. Root provides the Gravity theme + shared layout chrome.
export const routes: RouteRecord[] = [
  {
    path: "/",
    element: <Root />,
    children: [
      { index: true, element: <Home /> },
      { path: "features", element: <Features /> },
      { path: "installation", element: <Installation /> },
      { path: "tmux-setup", element: <TmuxSetup /> },
      { path: "cli", element: <Cli /> },
      { path: "configuration", element: <Configuration /> },
      { path: "tui-picker", element: <TuiPicker /> },
      { path: "about", element: <About /> },
    ],
  },
];

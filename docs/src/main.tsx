import { ViteReactSSG } from "vite-react-ssg";
import { routes } from "./routes";

// Gravity UI base styles first, then our brand overrides on top.
import "@gravity-ui/uikit/styles/styles.css";
import "./styles.css";

// Statically pre-render every route to HTML (SSG) so per-page SEO and the
// content are present without JavaScript. dirStyle "nested" keeps the existing
// /features/ -> /features/index.html URLs intact.
export const createRoot = ViteReactSSG({ routes });

import { ThemeProvider } from "@gravity-ui/uikit";
import { Layout } from "./Layout";

// Root wraps the whole site in the Gravity UI dark theme and the shared layout.
// It is the element of the "/" route; every page renders through Layout's
// <Outlet/>.
export function Root() {
  return (
    <ThemeProvider theme="dark">
      <Layout />
    </ThemeProvider>
  );
}

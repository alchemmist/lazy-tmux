import {
  ThemeProvider,
  Toaster,
  ToasterComponent,
  ToasterProvider,
} from "@gravity-ui/uikit";
import { Layout } from "./Layout";

// A single Toaster instance powers the "Copied" notifications fired from the
// code components.
const toaster = new Toaster();

// Root wraps the whole site in the Gravity UI dark theme, the toaster context
// and the shared layout. It is the element of the "/" route; every page renders
// through Layout's <Outlet/>.
export function Root() {
  return (
    <ThemeProvider theme="dark">
      <ToasterProvider toaster={toaster}>
        <Layout />
        <ToasterComponent />
      </ToasterProvider>
    </ThemeProvider>
  );
}

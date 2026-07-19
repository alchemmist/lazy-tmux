import { useState } from "react";
import { NavLink, Outlet, useLocation } from "react-router-dom";
import { AsciiField } from "./AsciiField";
import { Version } from "./Version";

const NAV: { label: string; to: string }[] = [
  { label: "Home", to: "/" },
  { label: "Features", to: "/features" },
  { label: "Installation", to: "/installation" },
  { label: "tmux setup", to: "/tmux-setup" },
  { label: "CLI", to: "/cli" },
  { label: "Configuration", to: "/configuration" },
  { label: "TUI picker", to: "/tui-picker" },
  { label: "Integrations", to: "/integrations" },
  { label: "About", to: "/about" },
];

const GITHUB = "https://github.com/alchemmist/lazy-tmux";

export function Layout() {
  const [navOpen, setNavOpen] = useState(false);
  const location = useLocation();
  const isHome = location.pathname === "/";
  const closeNav = () => setNavOpen(false);

  return (
    <div className={`shell${navOpen ? " nav-open" : ""}`}>
      {/* ASCII background on every page. Rendered here (outside
          .content/.route-fade) so no transformed ancestor confines the fixed
          canvas to a sub-box. "docs" mode keeps a clear reading column. */}
      <AsciiField mode={isHome ? "home" : "docs"} />
      <header className="topbar">
        <button
          type="button"
          className="nav-toggle"
          aria-label="Toggle menu"
          aria-expanded={navOpen}
          onClick={() => setNavOpen((open) => !open)}
        >
          ☰
        </button>
        <NavLink to="/" className="topbar-brand" onClick={closeNav}>
          lazy-tmux
        </NavLink>
      </header>

      <aside className="sidebar">
        <NavLink to="/" className="brand" onClick={closeNav}>
          <img src="/assets/logo-white.svg" alt="lazy-tmux logo" />
          <span>lazy-tmux</span>
        </NavLink>
        <Version />
        <nav>
          {NAV.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.to === "/"}
              className={({ isActive }) => (isActive ? "active" : undefined)}
              onClick={closeNav}
            >
              {item.label}
            </NavLink>
          ))}
          <a className="external" href={GITHUB} target="_blank" rel="noopener noreferrer">
            GitHub ↗
          </a>
        </nav>
      </aside>

      {/* Click-away backdrop for the mobile drawer. */}
      <div className="nav-backdrop" onClick={closeNav} aria-hidden="true" />

      <main className={`content${isHome ? " home" : ""}`}>
        <div className="page">
          {/* key by path so the route content re-runs its fade-in on navigation */}
          <div className="route-fade" key={location.pathname}>
            <Outlet />
          </div>
          <Footer />
        </div>
      </main>
    </div>
  );
}

function Footer() {
  return (
    <footer className="site-footer">
      <p className="muted">
        Author: Anton Grishin (alchemmist), <code>anton.ingrish@gmail.com</code>
      </p>
      <p className="muted">
        OpenSource:{" "}
        <a href={GITHUB}>alchemmist/lazy-tmux</a>
      </p>
      <p className="muted">© 2026 lazy-tmux</p>
    </footer>
  );
}

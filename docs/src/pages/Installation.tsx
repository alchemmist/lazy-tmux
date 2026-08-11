import { Alert } from "@gravity-ui/uikit";
import { Seo } from "../components/Seo";
import { CodeBlock } from "../components/CodeBlock";
import { InlineCode } from "../components/InlineCode";

export function Installation() {
  return (
    <>
      <Seo
        title="Installation — lazy-tmux"
        description="Install lazy-tmux via the install script, Arch (AUR) or Homebrew."
        slug="installation"
      />
      <section className="doc-section">
        <h1>Installation</h1>

        <Alert
          theme="info"
          align="center"
          className="version-alert"
          title="tmux versions"
          message={
            <>
              lazy-tmux supports every tmux release from <strong>2.9</strong>{" "}
              through <strong>3.7b</strong>, verified on each one by the CI version
              matrix. Newer releases are added as they ship.
            </>
          }
        />

        <p>Install with builtin powerful TUI picker:</p>
        <CodeBlock>curl -fsSL https://lazy-tmux.xyz/install.sh | sh</CodeBlock>

        <p>
          Install pure, no-deps, lightweight binary (
          <InlineCode>fzf</InlineCode> required):
        </p>
        <CodeBlock>
          curl -fsSL https://lazy-tmux.xyz/install.sh | sh -s -- --fzf-engine
        </CodeBlock>

        <p>Or use your package manager.</p>
        <p>Arch:</p>
        <CodeBlock>yay -S lazy-tmux</CodeBlock>

        <p>macOS (Homebrew):</p>
        <CodeBlock>brew install lazy-tmux</CodeBlock>
      </section>
    </>
  );
}

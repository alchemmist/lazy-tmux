import { Head } from "vite-react-ssg";

const SITE = "https://lazy-tmux.xyz";
const OG_IMAGE = `${SITE}/assets/banner.png`;

const LD_JSON = {
  "@context": "https://schema.org",
  "@type": "SoftwareApplication",
  name: "lazy-tmux",
  description:
    "Lazy tmux session manager with scrollback restore, alternative to tmux-resurrect and tmux-continuum without bugs",
  applicationCategory: "Utility",
  operatingSystem: "Linux, macOS, BSD",
  url: SITE,
  sameAs: "https://github.com/alchemmist/lazy-tmux",
  downloadUrl: "https://github.com/alchemmist/lazy-tmux/releases",
  author: {
    "@type": "Person",
    name: "Anton Grishin (alchemmist)",
    email: "anton.ingrish@gmail.com",
  },
};

interface SeoProps {
  title: string;
  description: string;
  /** Path slug without slashes, e.g. "features"; empty string for home. */
  slug?: string;
  /** Emit the SoftwareApplication JSON-LD (home page only). */
  jsonLd?: boolean;
}

export function Seo({ title, description, slug = "", jsonLd = false }: SeoProps) {
  const canonical = slug ? `${SITE}/${slug}/` : `${SITE}/`;

  return (
    <Head>
      <title>{title}</title>
      <meta name="description" content={description} />
      <link rel="canonical" href={canonical} />
      <meta property="og:title" content={title} />
      <meta property="og:description" content={description} />
      <meta property="og:type" content="website" />
      <meta property="og:url" content={canonical} />
      <meta property="og:image" content={OG_IMAGE} />
      <meta property="og:image:width" content="1200" />
      <meta property="og:image:height" content="630" />
      <meta name="twitter:card" content="summary_large_image" />
      {jsonLd && (
        <script type="application/ld+json">{JSON.stringify(LD_JSON)}</script>
      )}
    </Head>
  );
}

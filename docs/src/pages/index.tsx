import type {ReactNode} from 'react';
import Link from '@docusaurus/Link';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import Layout from '@theme/Layout';

import styles from './index.module.css';

function HeroSection() {
  return (
    <section className={styles.hero}>
      <div className={styles.heroInner}>
        <h1 className={styles.title}>
          tv<span className={styles.titleAccent}>arr</span>
        </h1>
        <p className={styles.pronunciation}>/tee-vee-arr/</p>
        <p className={styles.tagline}>
          An IPTV proxy and stream aggregator for home users.
          Combine multiple sources into one curated manifest for your favorite player.
        </p>

        <div className={styles.buttons}>
          <Link className={styles.primaryBtn} to="/docs/next">
            Get Started
          </Link>
          <Link className={styles.secondaryBtn} to="https://github.com/jmylchreest/tvarr">
            View on GitHub
          </Link>
        </div>

        <div className={styles.codePreview}>
          <div className={styles.codeHeader}>
            <span className={`${styles.codeDot} ${styles.codeDotRed}`}></span>
            <span className={`${styles.codeDot} ${styles.codeDotYellow}`}></span>
            <span className={`${styles.codeDot} ${styles.codeDotGreen}`}></span>
            <span className={styles.codeTitle}>docker-compose.yml</span>
          </div>
          <div className={styles.codeContent}>
            <div className={styles.codeLine}>
              <span className={styles.codeKey}>services</span><span className={styles.codePunct}>:</span>
            </div>
            <div className={styles.codeLine}>
              <span className={styles.codeIndent}>  </span><span className={styles.codeKey}>tvarr</span><span className={styles.codePunct}>:</span>
            </div>
            <div className={styles.codeLine}>
              <span className={styles.codeIndent}>    </span><span className={styles.codeKey}>image</span><span className={styles.codePunct}>:</span> <span className={styles.codeValue}>ghcr.io/jmylchreest/tvarr:release</span>
            </div>
            <div className={styles.codeLine}>
              <span className={styles.codeIndent}>    </span><span className={styles.codeKey}>ports</span><span className={styles.codePunct}>:</span>
            </div>
            <div className={styles.codeLine}>
              <span className={styles.codeIndent}>      </span><span className={styles.codePunct}>-</span> <span className={styles.codeValue}>"8080:8080"</span>
            </div>
            <div className={styles.codeLine}>
              <span className={styles.codeIndent}>    </span><span className={styles.codeKey}>volumes</span><span className={styles.codePunct}>:</span>
            </div>
            <div className={styles.codeLine}>
              <span className={styles.codeIndent}>      </span><span className={styles.codePunct}>-</span> <span className={styles.codeValue}>./data:/data</span>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}

type FeatureItem = {
  icon: string;
  title: string;
  description: string;
};

const features: FeatureItem[] = [
  {
    icon: '',
    title: 'Aggregate Multiple Sources',
    description: 'Import from M3U files, URLs, or Xtream Codes APIs. Merge EPG data from XMLTV sources into unified playlists.',
  },
  {
    icon: '',
    title: 'Filter & Transform',
    description: 'Powerful expression-based rules to include/exclude channels. Data mapping transforms names, logos, and groups.',
  },
  {
    icon: '',
    title: 'Transcode On-Demand',
    description: 'Real-time transcoding with FFmpeg. Hardware acceleration with VAAPI, NVENC, QSV, and AMF support.',
  },
  {
    icon: '',
    title: 'Multiple Output Formats',
    description: 'Serve HLS, DASH, or raw MPEG-TS. Client detection serves appropriate quality profiles per device.',
  },
  {
    icon: '',
    title: 'Distributed Workers',
    description: 'Scale transcoding across multiple machines. Main node coordinates, workers process streams.',
  },
  {
    icon: '',
    title: 'Modern Web UI',
    description: 'Intuitive dashboard for managing sources, channels, and rules. Real-time status and job monitoring.',
  },
];

function FeaturesSection() {
  return (
    <section className={styles.features}>
      <div className={styles.featuresInner}>
        <h2 className={styles.featuresTitle}>Built for IPTV Enthusiasts</h2>
        <div className={styles.featuresGrid}>
          {features.map((feature, idx) => (
            <div key={idx} className={styles.featureCard}>
              <div className={styles.featureIcon}>{feature.icon}</div>
              <h3 className={styles.featureTitle}>{feature.title}</h3>
              <p className={styles.featureDesc}>{feature.description}</p>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}

function InstallSection() {
  return (
    <section className={styles.install}>
      <div className={styles.installInner}>
        <h2 className={styles.installTitle}>Quick Start</h2>
        <div className={styles.installCode}>
          <span>
            <span className={styles.installPrompt}>$ </span>
            docker compose up -d
          </span>
        </div>
        <p style={{marginTop: '1rem', color: '#666', fontSize: '0.875rem'}}>
          See <Link to="/docs/next/quickstart">Quickstart Guide</Link> for Docker Compose and Kubernetes options
        </p>
      </div>
    </section>
  );
}

export default function Home(): ReactNode {
  const {siteConfig} = useDocusaurusContext();
  return (
    <Layout
      title="Home"
      description={siteConfig.tagline}>
      <HeroSection />
      <FeaturesSection />
      <InstallSection />
    </Layout>
  );
}

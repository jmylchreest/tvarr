import React from 'react';
import clsx from 'clsx';
import Link from '@docusaurus/Link';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import Layout from '@theme/Layout';
import styles from './index.module.css';

function HomepageHeader() {
  const { siteConfig } = useDocusaurusContext();
  return (
    <header className={clsx('hero hero--primary', styles.heroBanner)}>
      <div className="container">
        <h1 className="hero__title">{siteConfig.title}</h1>
        <p className="hero__subtitle">{siteConfig.tagline}</p>
        <div className={styles.buttons}>
          <Link
            className="button button--secondary button--lg"
            to="/docs/next/">
            Get Started
          </Link>
          <Link
            className="button button--outline button--lg"
            style={{ marginLeft: '1rem', color: 'white', borderColor: 'white' }}
            href="https://github.com/jmylchreest/tvarr">
            View on GitHub
          </Link>
        </div>
      </div>
    </header>
  );
}

const features = [
  {
    title: 'Aggregate Streams',
    description: 'Combine multiple M3U and Xtream sources into unified playlists. Merge EPG data from XMLTV sources.',
  },
  {
    title: 'Filter & Transform',
    description: 'Powerful expression-based rules to include/exclude channels and transform metadata.',
  },
  {
    title: 'Transcode On-Demand',
    description: 'Real-time transcoding with FFmpeg. Hardware acceleration with VAAPI, NVENC, QSV, and AMF.',
  },
];

function Feature({ title, description }) {
  return (
    <div className={clsx('col col--4')}>
      <div className="padding-horiz--md padding-vert--lg">
        <h3>{title}</h3>
        <p>{description}</p>
      </div>
    </div>
  );
}

export default function Home(): JSX.Element {
  const { siteConfig } = useDocusaurusContext();
  return (
    <Layout
      title={siteConfig.title}
      description="IPTV Proxy & Stream Aggregator">
      <HomepageHeader />
      <main>
        <section className={styles.features}>
          <div className="container">
            <div className="row">
              {features.map((props, idx) => (
                <Feature key={idx} {...props} />
              ))}
            </div>
          </div>
        </section>
      </main>
    </Layout>
  );
}

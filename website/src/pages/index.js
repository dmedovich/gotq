import Link from '@docusaurus/Link';
import CodeBlock from '@theme/CodeBlock';
import Layout from '@theme/Layout';
import styles from './index.module.css';

const SAMPLE = `base := db.Where("tenant_id = ?", tenantID)

page, err := users.
    From(base).
    List(r.Context(), r.URL.Query())
if err != nil {
    queryhttp.WriteError(w, err)
    return
}

json.NewEncoder(w).Encode(page)`;

const FEATURES = [
  {
    title: 'Schema first',
    body: 'Public names, Go fields, database columns, scalar types, and allowed operations are explicit.',
  },
  {
    title: 'SQL-safe by construction',
    body: 'Values stay bound parameters. Columns come from an immutable whitelist and are dialect-quoted by GORM.',
  },
  {
    title: 'Errors clients can use',
    body: 'Stable error codes, field and operator metadata, and byte-accurate source positions.',
  },
  {
    title: 'Bounded work',
    body: 'Limits cover page size, offsets, input bytes, AST nodes, expression depth, and relationship traversal.',
  },
];

export default function Home() {
  return (
    <Layout
      title="Schema-first queries for GORM"
      description="Safe, schema-validated HTTP query parameters for GORM."
    >
      <header className={styles.hero}>
        <div className={`container ${styles.heroGrid}`}>
          <div>
            <div className={styles.eyebrow}>Go · HTTP · GORM</div>
            <h1 className={styles.title}>Query APIs without exposing SQL.</h1>
            <p className={styles.subtitle}>
              Parse a small HTTP query language, validate every field and literal
              against your model, then apply safe GORM scopes.
            </p>
            <div className={styles.actions}>
              <Link className="button button--primary button--lg" to="/docs/getting-started">
                Get started
              </Link>
              <Link className="button button--secondary button--lg" to="/docs/query-language">
                Query language
              </Link>
            </div>
          </div>
          <div className={styles.code}>
            <CodeBlock language="go">{SAMPLE}</CodeBlock>
          </div>
        </div>
      </header>
      <main className={styles.features}>
        <div className={`container ${styles.featureGrid}`}>
          {FEATURES.map((feature) => (
            <article className={styles.card} key={feature.title}>
              <h2>{feature.title}</h2>
              <p>{feature.body}</p>
            </article>
          ))}
        </div>
      </main>
    </Layout>
  );
}

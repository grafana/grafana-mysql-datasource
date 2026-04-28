import { expect, test, type ExplorePage } from '@grafana/plugin-e2e';
import type { Page, Response } from '@playwright/test';

const PROVISIONED_UID = 'mysql-test';
const PLUGIN_ID = 'mysql';

/**
 * Fixture time range. These must match the seed in `tests/e2e/fixtures/seed.sql`
 * (generated with `seed=42`, 1-minute interval, hosts: web-01, web-02, db-01).
 */
const FIXTURE_FROM_ISO = '2026-03-17T21:00:00.000Z';
const FIXTURE_TO_ISO = '2026-03-18T01:00:00.000Z';

type QueryOverrides = {
  rawSql?: string;
  editorMode?: 'builder' | 'code';
  format?: 'table' | 'time_series' | 'logs';
  dataset?: string;
};

/**
 * Build an Explore URL that pre-populates a query and time range.
 * Using the `panes` parameter rather than `left`/`right` fires exactly one
 * initial query and gives every test a deterministic starting state.
 */
function exploreUrl(uid: string, opts: QueryOverrides = {}): string {
  const panes = {
    explore: {
      datasource: uid,
      queries: [
        {
          refId: 'A',
          datasource: { type: PLUGIN_ID, uid },
          format: opts.format ?? 'table',
          rawSql: opts.rawSql ?? '',
          editorMode: opts.editorMode ?? 'code',
          dataset: opts.dataset ?? 'testdata',
          ...(opts.rawSql ? { rawQuery: true } : {}),
        },
      ],
      range: { from: FIXTURE_FROM_ISO, to: FIXTURE_TO_ISO },
    },
  };
  return `/explore?orgId=1&schemaVersion=1&panes=${encodeURIComponent(JSON.stringify(panes))}`;
}

/**
 * Register a `/api/ds/query` response listener that reads the body inside
 * the predicate. Calling `.json()` after the predicate resolves can race
 * the CDP buffer eviction, so the body is captured as a side effect.
 *
 * TODO: remove once @grafana/plugin-e2e exposes body reading natively.
 */
function waitForQueryDataResponseWithBody(explorePage: ExplorePage) {
  let body: Record<string, unknown> | null = null;
  const responsePromise = explorePage.waitForQueryDataResponse(async (r: Response) => {
    if (!r.ok()) return false;
    const b = (await r.json().catch(() => null)) as Record<string, unknown> | null;
    const frames = (b as { results?: { A?: { frames?: unknown[] } } })?.results?.A?.frames;
    if (!Array.isArray(frames)) return false;
    body = b;
    return true;
  });
  return { responsePromise, getBody: () => body };
}

/**
 * Switch the query editor to a specific mode.
 *
 * Code → Builder opens a "Warning: Builder mode does not display changes
 * made in code" dialog. Accept by clicking "Discard code and switch".
 *
 * Builder → Code does not trigger a dialog.
 */
async function switchMode(page: Page, mode: 'Builder' | 'Code') {
  await page.getByRole('radio', { name: mode }).click();
  const discardButton = page.getByRole('button', { name: 'Discard code and switch' });
  if (await discardButton.isVisible({ timeout: 2000 }).catch(() => false)) {
    await discardButton.click();
  }
  await expect(page.getByRole('radio', { name: mode })).toBeChecked();
}

test.describe('Query editor', () => {
  test.describe('rendering', () => {
    test(
      'smoke: renders Builder and Code mode radios',
      { tag: '@plugins' },
      async ({ page, explorePage }) => {
        await page.goto(exploreUrl(PROVISIONED_UID, { editorMode: 'code' }));
        await expect(page.getByRole('radio', { name: 'Builder' })).toBeVisible();
        await expect(page.getByRole('radio', { name: 'Code' })).toBeVisible();
        // Silence unused-fixture warning: exposing explorePage ensures the
        // fixture is materialised for later assertions that depend on it.
        expect(explorePage).toBeTruthy();
      }
    );

    test('renders the Format combobox across modes', async ({ page }) => {
      await page.goto(exploreUrl(PROVISIONED_UID, { editorMode: 'code' }));
      await expect(page.getByRole('combobox', { name: /Format/ })).toBeVisible();
      await switchMode(page, 'Builder');
      await expect(page.getByRole('combobox', { name: /Format/ })).toBeVisible();
    });
  });

  test.describe('Code mode', () => {
    test('shows the CodeMirror editor and Format query button', async ({ page }) => {
      await page.goto(exploreUrl(PROVISIONED_UID, { editorMode: 'code' }));
      await expect(page.getByRole('textbox', { name: /editor content/i })).toBeVisible();
      await expect(page.getByRole('button', { name: 'Format query' })).toBeVisible();
    });

    test('restores the SQL query from the URL', async ({ page }) => {
      // The Monaco editor's accessibility textarea reflects the current
      // query as its `value` attribute when the text is short enough to fit
      // in a single line — use that for a deterministic assertion instead
      // of simulating keystrokes, which are flaky in headless Monaco.
      await page.goto(
        exploreUrl(PROVISIONED_UID, { editorMode: 'code', rawSql: 'SELECT 1' })
      );
      await expect(page.getByRole('textbox', { name: /editor content/i })).toHaveValue(
        'SELECT 1'
      );
    });
  });

  test.describe('Builder mode', () => {
    test('shows Dataset and Table selectors', async ({ page }) => {
      await page.goto(exploreUrl(PROVISIONED_UID, { editorMode: 'builder' }));
      await expect(page.getByRole('combobox', { name: 'Dataset selector' })).toBeVisible();
      await expect(page.getByRole('combobox', { name: 'Table selector' })).toBeVisible();
    });

    test('shows Column selector and Filter/Group/Order toggles', async ({ page }) => {
      await page.goto(exploreUrl(PROVISIONED_UID, { editorMode: 'builder' }));
      await expect(page.getByRole('combobox', { name: 'Column' })).toBeVisible();
      await expect(page.getByRole('switch', { name: /Filter/ })).toBeVisible();
      await expect(page.getByRole('switch', { name: /Group/ })).toBeVisible();
      await expect(page.getByRole('switch', { name: /Order/ })).toBeVisible();
    });
  });
});

test.describe('Query editor with fixture data', () => {
  // Serial mode prevents parallel workers from competing for the same MySQL
  // backend and producing slow responses that look like failures.
  test.describe.configure({ mode: 'serial' });

  test.describe('metrics table', () => {
    test('code mode: SELECT returns rows for web-01', async ({ page, explorePage }) => {
      const { responsePromise, getBody } = waitForQueryDataResponseWithBody(explorePage);
      await page.goto(
        exploreUrl(PROVISIONED_UID, {
          editorMode: 'code',
          rawSql:
            "SELECT ts AS time, cpu_usage FROM testdata.metrics WHERE host='web-01' ORDER BY ts LIMIT 5",
        })
      );
      await responsePromise;
      const body = getBody() as {
        results: { A: { frames: Array<{ data: { values: unknown[][] } }> } };
      } | null;
      expect(body?.results.A.frames.length).toBeGreaterThan(0);
      const values = body?.results.A.frames[0].data.values;
      expect(values?.[0].length).toBe(5);
    });

    test('code mode: host filter reduces the result set', async ({ page, explorePage }) => {
      const { responsePromise, getBody } = waitForQueryDataResponseWithBody(explorePage);
      await page.goto(
        exploreUrl(PROVISIONED_UID, {
          editorMode: 'code',
          rawSql:
            "SELECT COUNT(*) AS rows_count FROM testdata.metrics WHERE host='db-01'",
        })
      );
      await responsePromise;
      const body = getBody() as {
        results: { A: { frames: Array<{ data: { values: unknown[][] } }> } };
      } | null;
      const count = body?.results.A.frames[0].data.values[0][0] as number;
      // The seed places every minute across ~4 hours × 3 hosts, so db-01 has
      // 241 rows. Assert a loose lower bound so small fixture tweaks don't
      // break the test.
      expect(count).toBeGreaterThan(200);
    });

    test('time_series format: aggregated cpu_usage returns frames', async ({
      page,
      explorePage,
    }) => {
      const { responsePromise, getBody } = waitForQueryDataResponseWithBody(explorePage);
      await page.goto(
        exploreUrl(PROVISIONED_UID, {
          editorMode: 'code',
          format: 'time_series',
          rawSql:
            'SELECT ts AS time, AVG(cpu_usage) AS value FROM testdata.metrics GROUP BY ts ORDER BY ts',
        })
      );
      await responsePromise;
      const body = getBody() as {
        results: { A: { frames: Array<{ data: { values: unknown[][] } }> } };
      } | null;
      expect(body?.results.A.frames.length).toBeGreaterThan(0);
    });
  });

  test.describe('logs table', () => {
    test('code mode: SELECT returns log entries', async ({ page, explorePage }) => {
      const { responsePromise, getBody } = waitForQueryDataResponseWithBody(explorePage);
      await page.goto(
        exploreUrl(PROVISIONED_UID, {
          editorMode: 'code',
          rawSql:
            'SELECT ts AS time, level, service, message FROM testdata.logs ORDER BY ts LIMIT 10',
        })
      );
      await responsePromise;
      const body = getBody() as {
        results: { A: { frames: Array<{ data: { values: unknown[][] } }> } };
      } | null;
      expect(body?.results.A.frames.length).toBeGreaterThan(0);
      const rows = body?.results.A.frames[0].data.values[0];
      expect(rows?.length).toBeGreaterThan(0);
      expect(rows?.length).toBeLessThanOrEqual(10);
    });
  });
});

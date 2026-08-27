import { expect, test } from '@grafana/plugin-e2e';

import { type MySQLOptions } from '../../src/types';

const PROVISIONED_FILE = 'datasources.yml';

/**
 * Connection details for the MySQL backend.
 *
 * Locally, the MySQL service runs under its Docker Compose service name
 * (`mysql`) and is reachable from inside the Grafana container at
 * `mysql:3306`. In Cloud end-to-end runs, the provisioned instance's
 * connection details are injected via `DS_INSTANCE_URL` (and other
 * `DS_INSTANCE_*` variables) by Grafana Bench.
 */
const [DS_HOST, DS_PORT] = (process.env.DS_INSTANCE_URL ?? 'mysql:3306').split(':');
const DS_USER = process.env.DS_INSTANCE_USERNAME ?? 'grafana';
const DS_PASSWORD = process.env.DS_INSTANCE_PASSWORD ?? 'grafana';
const DS_DATABASE = process.env.DS_INSTANCE_DATABASE ?? 'testdata';

test.describe('Config editor', () => {
  test.describe('rendering', () => {
    test('smoke: should render config editor', { tag: '@plugins' }, async ({ createDataSourceConfigPage, page }) => {
      await createDataSourceConfigPage({ type: 'mysql' });
      await expect(page.getByRole('heading', { name: 'Connection', exact: true })).toBeVisible();
      await expect(page.getByRole('heading', { name: 'Authentication' })).toBeVisible();
    });

    test('should render Connection section fields', async ({ createDataSourceConfigPage, page }) => {
      await createDataSourceConfigPage({ type: 'mysql' });
      await expect(page.getByRole('heading', { name: 'Connection', exact: true })).toBeVisible();
      await expect(page.getByPlaceholder('localhost:3306')).toBeVisible();
      await expect(page.getByPlaceholder('Database')).toBeVisible();
    });

    test('should render Authentication section fields', async ({ createDataSourceConfigPage, page }) => {
      await createDataSourceConfigPage({ type: 'mysql' });
      await expect(page.getByRole('heading', { name: 'Authentication' })).toBeVisible();
      await expect(page.getByPlaceholder('Username')).toBeVisible();
      await expect(page.getByPlaceholder('Password')).toBeVisible();
      // Labels and description spans both render this text — use .first() to
      // disambiguate for the strict-mode locator.
      await expect(page.getByText('Use TLS Client Auth').first()).toBeVisible();
      await expect(page.getByText('Skip TLS Verification').first()).toBeVisible();
    });

    test('should render Additional settings section', async ({ createDataSourceConfigPage, page }) => {
      await createDataSourceConfigPage({ type: 'mysql' });
      await expect(page.getByRole('heading', { name: 'Additional settings' })).toBeVisible();
      await expect(page.getByText('Session timezone').first()).toBeVisible();
      await expect(page.getByText('Min time interval').first()).toBeVisible();
      await expect(page.getByRole('heading', { name: 'Connection limits' })).toBeVisible();
    });
  });

  test.describe('provisioned datasource', () => {
    test('should load provisioned connection fields', async ({
      readProvisionedDataSource,
      gotoDataSourceConfigPage,
      page,
    }) => {
      const ds = await readProvisionedDataSource<MySQLOptions>({ fileName: PROVISIONED_FILE });
      await gotoDataSourceConfigPage(ds.uid);
      await expect(page.getByPlaceholder('localhost:3306')).toHaveValue(`${DS_HOST}:${DS_PORT}`);
      await expect(page.getByPlaceholder('Database')).toHaveValue(DS_DATABASE);
    });

    test('should load provisioned authentication fields', async ({
      readProvisionedDataSource,
      gotoDataSourceConfigPage,
      page,
    }) => {
      const ds = await readProvisionedDataSource<MySQLOptions>({ fileName: PROVISIONED_FILE });
      await gotoDataSourceConfigPage(ds.uid);
      await expect(page.getByPlaceholder('Username')).toHaveValue(DS_USER);
      // Secure field renders a masked placeholder indicating it is set, not
      // the actual password value.
      await expect(page.getByPlaceholder('Password')).toHaveValue('configured');
    });
  });

  test.describe('save & test', () => {
    test('should pass health check for provisioned datasource', async ({
      readProvisionedDataSource,
      gotoDataSourceConfigPage,
      page,
    }) => {
      const ds = await readProvisionedDataSource<MySQLOptions>({ fileName: PROVISIONED_FILE });
      const configPage = await gotoDataSourceConfigPage(ds.uid);
      // The provisioned datasource is marked `editable: true`, so Grafana
      // renders "Save & test" rather than the read-only "Test" button. Click
      // it directly rather than using `configPage.saveAndTest()` so the
      // behaviour is the same whether or not the datasource is editable.
      await page.getByRole('button', { name: /^(Save & test|Test)$/ }).click();
      await expect(configPage).toHaveAlert('success', { hasText: 'Database Connection OK', timeout: 15000 });
    });

    test('should show error alert when credentials are invalid', async ({ createDataSourceConfigPage, page }) => {
      const configPage = await createDataSourceConfigPage({ type: 'mysql' });
      await page.getByPlaceholder('localhost:3306').fill(`${DS_HOST}:${DS_PORT}`);
      await page.getByPlaceholder('Database').fill(DS_DATABASE);
      await page.getByPlaceholder('Username').fill(DS_USER);
      await page.getByPlaceholder('Password').fill('wrong-password');
      await configPage.saveAndTest();
      await expect(configPage).toHaveAlert('error', {
        hasText:
          '[auth] MySQL rejected the configured account. Verify the username, password, and account access. MySQL error number: 1045.',
        timeout: 15000,
      });
    });

    test('should show error alert when backend is unreachable', async ({ createDataSourceConfigPage, page }) => {
      const configPage = await createDataSourceConfigPage({ type: 'mysql' });
      await page.getByPlaceholder('localhost:3306').fill('unreachable.invalid:3306');
      await page.getByPlaceholder('Username').fill(DS_USER);
      await page.getByPlaceholder('Password').fill(DS_PASSWORD);
      await configPage.saveAndTest();
      await expect(configPage).toHaveAlert('error', {
        hasText:
          '[network] Grafana could not establish a connection to MySQL. Verify the hostname, port, and network access from the Grafana server. The configured hostname or service name could not be resolved.',
        timeout: 30000,
      });
    });
  });
});

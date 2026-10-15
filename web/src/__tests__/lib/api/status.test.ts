/** @jest-environment node */
import { http, HttpResponse } from 'msw';
import { setupServer } from 'msw/node';
import { getWebStatus, webdavInternalUrl } from '@/lib/api/status';
import { systemStatus } from '@/mocks/fixtures/system';

const BACKEND = 'http://backend.test';
const server = setupServer();

beforeAll(() => {
  process.env.MOMOSHTREM_URL = BACKEND;
  process.env.BACKEND_API_TOKEN = 'shared-secret';
  delete process.env.WEBDAV_INTERNAL_URL;
  server.listen({ onUnhandledRequest: 'error' });
});
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

describe('getWebStatus', () => {
  it('passes the backend status through and adds the web version', async () => {
    let token: string | null = null;
    server.use(
      http.get(`${BACKEND}/api/status`, ({ request }) => {
        token = request.headers.get('x-momoshtrem-token');
        return HttpResponse.json(systemStatus());
      })
    );
    const result = await getWebStatus(false);
    expect(token).toBe('shared-secret');
    expect(result.ok && result.status.version).toBe(systemStatus().version);
    expect(result.ok && result.status.web?.version).toMatch(/^\d+\.\d+\.\d+/);
  });

  it('asks the backend to re-check on refresh', async () => {
    let url = '';
    server.use(
      http.get(`${BACKEND}/api/status`, ({ request }) => {
        url = request.url;
        return HttpResponse.json(systemStatus());
      })
    );
    await getWebStatus(true);
    expect(url).toBe(`${BACKEND}/api/status?refresh=1`);
  });

  it('reports an unreachable backend as 503 and says whether WebDAV still answers', async () => {
    server.use(
      http.get(`${BACKEND}/api/status`, () => HttpResponse.error()),
      http.options('http://backend.test:36911/', () => new HttpResponse(null, { status: 401 }))
    );
    const result = await getWebStatus(false);
    expect(result.ok).toBe(false);
    if (!result.ok) {
      expect(result.httpStatus).toBe(503);
      expect(result.body.error.code).toBe('backend_unreachable');
      expect(result.body.webdav).toEqual({ reachable: true });
    }
  });

  it('says WebDAV is down too when it does not answer', async () => {
    server.use(
      http.get(`${BACKEND}/api/status`, () => HttpResponse.error()),
      http.options('http://backend.test:36911/', () => HttpResponse.error())
    );
    const result = await getWebStatus(false);
    expect(!result.ok && result.body.webdav).toEqual({ reachable: false });
  });

  it('turns a rejected token into 502 backend_auth', async () => {
    server.use(http.get(`${BACKEND}/api/status`, () => HttpResponse.json({ error: { code: 'unauthorized', message: 'x' } }, { status: 401 })));
    const result = await getWebStatus(false);
    expect(!result.ok && result.httpStatus).toBe(502);
    expect(!result.ok && result.body.error.code).toBe('backend_auth');
  });

  it('derives the WebDAV address from the API host unless configured', () => {
    expect(webdavInternalUrl()).toBe('http://backend.test:36911/');
    process.env.WEBDAV_INTERNAL_URL = 'http://webdav.lan:8080/';
    expect(webdavInternalUrl()).toBe('http://webdav.lan:8080/');
    delete process.env.WEBDAV_INTERNAL_URL;
  });
});

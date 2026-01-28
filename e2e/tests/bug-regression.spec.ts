import { test, expect, Page, WebSocket } from '@playwright/test';
import * as path from 'path';
import {
  selectors,
  timeouts,
  UPLOAD_STAGES_COUNT,
  MIN_CLUSTER_NODES,
} from './selectors';

/**
 * Bug Regression Tests for OuroborosDB Dashboard
 *
 * These tests document expected behavior for known bugs.
 * They will FAIL until the underlying code is fixed.
 */

type DistributionTotals = {
  totalNodes: number;
  totalVertices: number;
  totalBlocks: number;
  totalSlices: number;
};

async function setup(page: Page): Promise<{ logWebSocket: WebSocket }> {
  const wsPromise = page.waitForEvent('websocket', {
    predicate: (ws) => ws.url().includes('/ws/logs'),
    timeout: timeouts.websocketConnect,
  });

  await page.goto('/');
  await expect(page.locator(selectors.connectionStatus)).toHaveText(
    'Connected',
    { timeout: timeouts.websocketConnect }
  );

  const logWebSocket = await wsPromise;
  return { logWebSocket };
}

async function parseCounterValue(
  page: Page,
  cardSelector: string
): Promise<number> {
  const card = page.locator(cardSelector);
  await expect(card).toBeVisible();
  const txt = (
    await card.locator(selectors.distribution.counterValue).first().textContent()
  )?.trim();
  return parseInt(txt || '0', 10);
}

async function getDistributionStats(page: Page): Promise<DistributionTotals> {
  const resp = await page.request.get('/api/distribution', {
    timeout: timeouts.apiResponse,
  });
  expect(resp.status()).toBe(200);

  const data = (await resp.json()) as {
    totalNodes?: number;
    totalVertices?: number;
    totalBlocks?: number;
    totalSlices?: number;
  };
  return {
    totalNodes: data.totalNodes || 0,
    totalVertices: data.totalVertices || 0,
    totalBlocks: data.totalBlocks || 0,
    totalSlices: data.totalSlices || 0,
  };
}

async function getVertexViaApi(
  page: Page,
  hash: string
): Promise<{ status: number; body: any }>
{
  const resp = await page.request.get(`/api/vertices/${hash}`, {
    timeout: timeouts.apiResponse,
  });

  let body: any = null;
  try {
    body = await resp.json();
  } catch {
    body = await resp.text();
  }

  return { status: resp.status(), body };
}

async function subscribeToNodeLogs(
  page: Page,
  nodeId: string
): Promise<{ status: number; body: any }>
{
  const resp = await page.request.post(`/api/nodes/${nodeId}/logs/subscribe`, {
    timeout: timeouts.apiResponse,
  });

  let body: any = null;
  try {
    body = await resp.json();
  } catch {
    body = await resp.text();
  }

  return { status: resp.status(), body };
}

async function unsubscribeFromNodeLogs(
  page: Page,
  nodeId: string
): Promise<{ status: number; body: any }>
{
  const resp = await page.request.post(
    `/api/nodes/${nodeId}/logs/unsubscribe`,
    { timeout: timeouts.apiResponse }
  );

  let body: any = null;
  try {
    body = await resp.json();
  } catch {
    body = await resp.text();
  }

  return { status: resp.status(), body };
}

async function waitForLogMessage(
  ws: WebSocket,
  timeoutMs: number
): Promise<any> {
  const frame = await ws.waitForEvent('framereceived', {
    timeout: timeoutMs,
  });
  try {
    return JSON.parse(frame.payload);
  } catch {
    return { raw: frame.payload };
  }
}

async function uploadReturnsFlowAndVertexHash(
  page: Page
): Promise<{ uploadID: string; vertexHash: string }> {
  await page.click(selectors.tabs.upload);

  const fileInput = page.locator(selectors.upload.fileInput);
  const testFilePath = path.join(__dirname, '..', 'fixtures', 'test-file.txt');

  const uploadRespPromise = page.waitForResponse(
    (r) => r.request().method() === 'POST' && r.url().includes('/api/upload'),
    { timeout: timeouts.apiResponse }
  );

  await fileInput.setInputFiles(testFilePath);
  const uploadResp = await uploadRespPromise;
  expect(uploadResp.status(), 'upload should be accepted').toBe(202);

  const uploadBody = (await uploadResp.json()) as { id?: string };
  const uploadID = uploadBody.id;
  if (!uploadID) {
    throw new Error('upload id missing from /api/upload response');
  }

  let vertexHashFromFlow = '';
  const flowURLPart = `/api/upload/${uploadID}/flow`;
  const onResp = async (resp: any) => {
    if (!resp.url().includes(flowURLPart)) {
      return;
    }
    try {
      const j = await resp.json();
      if (j && typeof j.vertexHash === 'string' && j.vertexHash) {
        vertexHashFromFlow = j.vertexHash;
      }
    } catch {
      // ignore
    }
  };

  page.on('response', onResp);

  const completedStages = page.locator(selectors.upload.flowStageComplete);
  await expect(completedStages).toHaveCount(UPLOAD_STAGES_COUNT, {
    timeout: timeouts.uploadComplete,
  });

  page.off('response', onResp);

  if (!vertexHashFromFlow) {
    const hashEl = page.locator(selectors.upload.hash).first();
    vertexHashFromFlow = (await hashEl.textContent())?.trim() || '';
  }
  if (!vertexHashFromFlow) {
    throw new Error('vertex hash missing from upload flow');
  }

  return { uploadID, vertexHash: vertexHashFromFlow };
}

async function getFirstNodeID(page: Page): Promise<string> {
  const resp = await page.request.get('/api/nodes', {
    timeout: timeouts.apiResponse,
  });
  expect(resp.status()).toBe(200);
  const nodes = (await resp.json()) as Array<{ nodeId?: string }>;
  if (!nodes.length || !nodes[0].nodeId) {
    throw new Error('no nodes returned from /api/nodes');
  }
  return nodes[0].nodeId;
}

test.describe('Bug Regression', () => {
  test.describe('Bug #1: upload then browser returns 404', () => {
    test('upload returns valid vertex hash', async ({ page }) => {
      await setup(page);
      const { vertexHash } = await uploadReturnsFlowAndVertexHash(page);

      // Expected behavior: a real deterministic CAS hash (hex), not a fake "vertex-...".
      expect(vertexHash.startsWith('vertex-')).toBe(false);
      expect(vertexHash).toMatch(/^[0-9a-f]{64}$/i);
    });

    test('uploaded vertex is retrievable via API', async ({ page }) => {
      await setup(page);
      const { vertexHash } = await uploadReturnsFlowAndVertexHash(page);

      const v = await getVertexViaApi(page, vertexHash);
      expect(v.status, 'vertex lookup should succeed').toBe(200);
    });

    test('uploaded vertex appears in browser search', async ({ page }) => {
      await setup(page);
      const { vertexHash } = await uploadReturnsFlowAndVertexHash(page);

      let dialogMessage = '';
      page.on('dialog', async (dialog) => {
        dialogMessage = dialog.message();
        await dialog.dismiss();
      });

      await page.click(selectors.tabs.data);
      await page.locator(selectors.data.vertexSearch).fill(vertexHash);

      const waitVertexResp = page.waitForResponse(
        (r) => r.url().includes(`/api/vertices/${vertexHash}`),
        { timeout: timeouts.apiResponse }
      );
      await page.click(selectors.data.searchButton);
      const vertexResp = await waitVertexResp;

      expect(vertexResp.status(), 'vertex browser should not return 404').toBe(200);
      await expect
        .poll(() => dialogMessage, { timeout: timeouts.apiResponse })
        .toContain(vertexHash);
    });
  });

  test.describe('Bug #2: node log subscription returns 500', () => {
    test('subscription API returns 200', async ({ page }) => {
      await setup(page);
      const nodeId = await getFirstNodeID(page);
      const res = await subscribeToNodeLogs(page, nodeId);
      expect(res.status, 'subscribe should not 500').toBe(200);
    });

    test('unsubscribe works correctly', async ({ page }) => {
      await setup(page);
      const nodeId = await getFirstNodeID(page);
      const sub = await subscribeToNodeLogs(page, nodeId);
      expect(sub.status, 'subscribe should succeed before unsubscribe').toBe(200);

      const unsub = await unsubscribeFromNodeLogs(page, nodeId);
      expect(unsub.status, 'unsubscribe should return 200').toBe(200);
    });

    test('receives logs after subscription (WebSocket)', async ({ page }) => {
      const { logWebSocket } = await setup(page);
      const nodeId = await getFirstNodeID(page);
      const sub = await subscribeToNodeLogs(page, nodeId);
      expect(sub.status, 'subscribe should not 500').toBe(200);

      // Trigger activity (upload) that should generate logs.
      await uploadReturnsFlowAndVertexHash(page);

      // Expect at least one log frame to arrive via /ws/logs.
      const msg = await waitForLogMessage(logWebSocket, timeouts.logAppear);
      expect(msg).toBeTruthy();
      expect(msg.type).toBe('log');
    });
  });

  test.describe('Bug #3: distribution counters not working', () => {
    test('counters show values (may be zero) initially', async ({ page }) => {
      await setup(page);
      await page.click(selectors.tabs.distribution);

      const totalNodes = await parseCounterValue(
        page,
        selectors.distribution.totalNodesCard
      );
      const totalVertices = await parseCounterValue(
        page,
        selectors.distribution.totalVerticesCard
      );
      const totalBlocks = await parseCounterValue(
        page,
        selectors.distribution.totalBlocksCard
      );
      const totalSlices = await parseCounterValue(
        page,
        selectors.distribution.totalSlicesCard
      );

      expect(totalNodes).toBeGreaterThanOrEqual(MIN_CLUSTER_NODES);
      expect(totalVertices).toBeGreaterThanOrEqual(0);
      expect(totalBlocks).toBeGreaterThanOrEqual(0);
      expect(totalSlices).toBeGreaterThanOrEqual(0);
    });

    test('API returns non-zero values after upload', async ({ page }) => {
      await setup(page);
      await uploadReturnsFlowAndVertexHash(page);

      const totals = await getDistributionStats(page);
      expect(totals.totalNodes).toBeGreaterThanOrEqual(MIN_CLUSTER_NODES);
      expect(totals.totalVertices).toBeGreaterThan(0);
      expect(totals.totalBlocks).toBeGreaterThan(0);
      expect(totals.totalSlices).toBeGreaterThan(0);
    });

    test('vertex counter increments after upload (UI)', async ({ page }) => {
      await setup(page);
      const before = await getDistributionStats(page);
      await uploadReturnsFlowAndVertexHash(page);
      const after = await getDistributionStats(page);
      expect(after.totalVertices).toBeGreaterThan(before.totalVertices);
    });

    test('block counter increments after upload (UI)', async ({ page }) => {
      await setup(page);
      const before = await getDistributionStats(page);
      await uploadReturnsFlowAndVertexHash(page);
      const after = await getDistributionStats(page);
      expect(after.totalBlocks).toBeGreaterThan(before.totalBlocks);
    });

    test('slice counter increments after upload (UI)', async ({ page }) => {
      await setup(page);
      const before = await getDistributionStats(page);
      await uploadReturnsFlowAndVertexHash(page);
      const after = await getDistributionStats(page);
      expect(after.totalSlices).toBeGreaterThan(before.totalSlices);
    });

    test('distribution UI counters should be non-zero after upload', async ({ page }) => {
      await setup(page);
      await uploadReturnsFlowAndVertexHash(page);

      // Refresh distribution panel content by navigating to the tab.
      await page.click(selectors.tabs.distribution);
      await expect(page.locator(selectors.distribution.overview)).toBeVisible({
        timeout: timeouts.apiResponse,
      });

      const totalNodes = await parseCounterValue(
        page,
        selectors.distribution.totalNodesCard
      );
      const totalVertices = await parseCounterValue(
        page,
        selectors.distribution.totalVerticesCard
      );
      const totalBlocks = await parseCounterValue(
        page,
        selectors.distribution.totalBlocksCard
      );
      const totalSlices = await parseCounterValue(
        page,
        selectors.distribution.totalSlicesCard
      );

      expect(totalNodes).toBeGreaterThanOrEqual(MIN_CLUSTER_NODES);
      expect(totalVertices).toBeGreaterThan(0);
      expect(totalBlocks).toBeGreaterThan(0);
      expect(totalSlices).toBeGreaterThan(0);
    });
  });
});

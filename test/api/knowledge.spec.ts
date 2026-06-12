import { test, expect } from '@playwright/test';
import {
  requireAuth, getToken, authHeaders, authHeadersMultipart,
  assertSuccess, assertError, assertPaginatedResponse, assertFields,
  apiUrl, uniqueName, testArticleData,
} from '../utils/test-helpers.js';

/**
 * 知识库管理接口集成测试 — 全覆盖 19 个端点。
 *
 * 测试组织：按业务流程（知识库 CRUD → 文章 CRUD → 审核 → 发布/停用 → 文档上传）。
 */

// ==================== 知识库 CRUD ====================

test.describe('知识库 CRUD', () => {
  let kbId: number;

  test.describe('GET /api/v1/portal/knowledge-bases', () => {
    test('门户端返回知识库列表（仅 id + name，不含管理字段）', async ({ request }) => {
      const token = requireAuth();
      const resp = await request.get(apiUrl('/api/v1/portal/knowledge-bases'), {
        headers: authHeaders(token),
      });

      const body = await assertSuccess(resp);
      const data = body.data as Array<Record<string, unknown>>;
      expect(Array.isArray(data)).toBe(true);
      if (data.length > 0) {
        const item = data[0];
        assertFields(item, { id: 'number', name: 'string' });
        expect(item.embedding_model).toBeUndefined();
        expect(item.vector_dimension).toBeUndefined();
      }
    });

    test('无 token 访问返回 401', async ({ request }) => {
      const resp = await request.get(apiUrl('/api/v1/portal/knowledge-bases'));
      await assertError(resp, 401, 10001);
    });
  });

  test.describe('GET /api/v1/admin/knowledge-bases', () => {
    test('返回知识库列表（含所有管理字段）', async ({ request }) => {
      const token = requireAuth();
      const resp = await request.get(apiUrl('/api/v1/admin/knowledge-bases'), {
        headers: authHeaders(token),
      });

      const body = await assertSuccess(resp);
      const data = body.data as Record<string, unknown>;
      expect(data.items).toBeDefined();
      const items = data.items as Array<Record<string, unknown>>;
      expect(Array.isArray(items)).toBe(true);
      if (items.length > 0) {
        assertFields(items[0], {
          id: 'number', name: 'string', embedding_model: 'string',
          vector_dimension: 'number', article_count: 'number',
        });
      }
    });
  });

  test.describe('POST + PUT + DELETE /api/v1/admin/knowledge-bases', () => {
    test('创建→更新→删除知识库（完整生命周期）', async ({ request }) => {
      const token = requireAuth();
      const name = uniqueName('KB生命周期');

      // 创建
      const createResp = await request.post(apiUrl('/api/v1/admin/knowledge-bases'), {
        headers: authHeaders(token),
        data: { name, description: '生命周期测试', embedding_model: 'bge-m3', vector_dimension: 1024 },
      });
      const createBody = await assertSuccess(createResp);
      kbId = (createBody.data as Record<string, unknown>).id as number;
      expect(kbId).toBeGreaterThan(0);

      // 获取列表确认存在
      const listResp = await request.get(apiUrl('/api/v1/admin/knowledge-bases'), {
        headers: authHeaders(token),
      });
      const listBody = await listResp.json();
      const found = (listBody.data.items as Array<Record<string, unknown>>).find(
        (kb: Record<string, unknown>) => kb.id === kbId,
      );
      expect(found).toBeDefined();

      // 更新
      const newName = `${name}_updated`;
      const updateResp = await request.put(apiUrl(`/api/v1/admin/knowledge-bases/${kbId}`), {
        headers: authHeaders(token),
        data: { name: newName, description: '更新后' },
      });
      await assertSuccess(updateResp);

      // 删除
      const deleteResp = await request.delete(apiUrl(`/api/v1/admin/knowledge-bases/${kbId}`), {
        headers: authHeaders(token),
      });
      await assertSuccess(deleteResp);
    });

    test('更新不存在的知识库返回 404', async ({ request }) => {
      const token = requireAuth();
      const resp = await request.put(apiUrl('/api/v1/admin/knowledge-bases/99999'), {
        headers: authHeaders(token),
        data: { name: '不存在的KB' },
      });
      await assertError(resp, 200, 10004);
    });

    test('创建时缺少必填字段返回校验失败', async ({ request }) => {
      const token = requireAuth();
      const resp = await request.post(apiUrl('/api/v1/admin/knowledge-bases'), {
        headers: authHeaders(token),
        data: { name: '仅名称' },
      });
      await assertError(resp, 200, 10003);
    });
  });
});

// ==================== 知识文章 CRUD + 审核流程 ====================

test.describe('知识文章生命周期', () => {
  let kbId: number;
  let articleId: number;
  const token = getToken();

  test.beforeAll(async ({ request }) => {
    if (!token) return;
    // 获取或创建可用知识库
    const resp = await request.get(apiUrl('/api/v1/admin/knowledge-bases'), {
      headers: authHeaders(token),
    });
    const body = await resp.json();
    if (body.code === 0 && body.data?.items?.length > 0) {
      kbId = body.data.items[0].id;
    }
  });

  test('创建文章 → 提交审核 → 驳回（含审核意见）', async ({ request }) => {
    if (!token || !kbId) { test.skip(true, '缺少 token 或知识库'); return; }

    // 创建
    const data = testArticleData();
    const createResp = await request.post(apiUrl(`/api/v1/admin/knowledge-bases/${kbId}/articles`), {
      headers: authHeaders(token), data,
    });
    const createBody = await assertSuccess(createResp);
    articleId = (createBody.data as Record<string, unknown>).id as number;

    // 提交审核
    const submitResp = await request.post(apiUrl(`/api/v1/admin/articles/${articleId}/submit-review`), {
      headers: authHeaders(token),
    });
    await assertSuccess(submitResp);

    // 驳回（需填写审核意见）
    const reviewResp = await request.post(apiUrl(`/api/v1/admin/articles/${articleId}/review`), {
      headers: authHeaders(token),
      data: { approved: false, review_comment: '内容需要补充更多细节' },
    });
    await assertSuccess(reviewResp);

    // 验证状态为驳回(6)
    const detailResp = await request.get(apiUrl(`/api/v1/admin/articles/${articleId}`), {
      headers: authHeaders(token),
    });
    const detailBody = await detailResp.json();
    expect(detailBody.data.status).toBe(6);
  });

  test('创建缺少标题返回校验失败', async ({ request }) => {
    if (!token || !kbId) { test.skip(true, '缺少 token 或知识库'); return; }
    const resp = await request.post(apiUrl(`/api/v1/admin/knowledge-bases/${kbId}/articles`), {
      headers: authHeaders(token),
      data: { content: '只有内容没有标题' },
    });
    await assertError(resp, 200, 10003);
  });

  test('文章列表按状态筛选', async ({ request }) => {
    if (!token || !kbId) { test.skip(true, '缺少 token 或知识库'); return; }
    const resp = await request.get(
      apiUrl(`/api/v1/admin/knowledge-bases/${kbId}/articles?status=1`),
      { headers: authHeaders(token) },
    );
    await assertPaginatedResponse(resp);
  });

  test('不存在的文章返回 404', async ({ request }) => {
    if (!token) { test.skip(true, '缺少 token'); return; }
    const resp = await request.get(apiUrl('/api/v1/admin/articles/99999'), {
      headers: authHeaders(token),
    });
    await assertError(resp, 200, 10004);
  });
});

// ==================== 发布/停用/启用 ====================

test.describe('发布/停用/启用', () => {
  const token = getToken();

  test('发布不存在的文章返回 404', async ({ request }) => {
    if (!token) { test.skip(true, '缺少 token'); return; }
    const resp = await request.post(apiUrl('/api/v1/admin/articles/99999/publish'), {
      headers: authHeaders(token),
    });
    await assertError(resp, 200, 10004);
  });

  test('停用不存在的文章返回 404', async ({ request }) => {
    if (!token) { test.skip(true, '缺少 token'); return; }
    const resp = await request.post(apiUrl('/api/v1/admin/articles/99999/disable'), {
      headers: authHeaders(token),
    });
    await assertError(resp, 200, 10004);
  });

  test('retry-sync 不存在文章返回 404', async ({ request }) => {
    if (!token) { test.skip(true, '缺少 token'); return; }
    const resp = await request.post(apiUrl('/api/v1/admin/articles/99999/retry-sync'), {
      headers: authHeaders(token),
    });
    await assertError(resp, 200, 10004);
  });

  test('启用不存在的文章返回 404', async ({ request }) => {
    if (!token) { test.skip(true, '缺少 token'); return; }
    const resp = await request.post(apiUrl('/api/v1/admin/articles/99999/enable'), {
      headers: authHeaders(token),
    });
    await assertError(resp, 200, 10004);
  });
});

// ==================== 文档上传 ====================

test.describe('文档上传与处理', () => {
  let kbId: number;
  const token = getToken();

  test.beforeAll(async ({ request }) => {
    if (!token) return;
    const resp = await request.get(apiUrl('/api/v1/admin/knowledge-bases'), {
      headers: authHeaders(token),
    });
    const body = await resp.json();
    if (body.code === 0 && body.data?.items?.length > 0) {
      kbId = body.data.items[0].id;
    }
  });

  test('上传不支持的格式 (.exe) 返回校验失败', async ({ request }) => {
    if (!token || !kbId) { test.skip(true, '缺少 token 或知识库'); return; }

    const boundary = '----TestBoundary001';
    const body = [
      `--${boundary}`,
      'Content-Disposition: form-data; name="files"; filename="test.exe"',
      'Content-Type: application/octet-stream',
      '', 'fake exe content',
      `--${boundary}--`,
    ].join('\r\n');

    const resp = await request.post(
      apiUrl(`/api/v1/admin/knowledge-bases/${kbId}/documents/upload`),
      {
        headers: {
          ...authHeadersMultipart(token),
          'Content-Type': `multipart/form-data; boundary=${boundary}`,
        },
        data: body,
      },
    );
    await assertError(resp, 200, 10003);
  });

  test('上传不存在的知识库返回 404', async ({ request }) => {
    if (!token) { test.skip(true, '缺少 token'); return; }

    const boundary = '----TestBoundary002';
    const body = [
      `--${boundary}`,
      'Content-Disposition: form-data; name="files"; filename="test.md"',
      'Content-Type: text/markdown',
      '', '# 测试',
      `--${boundary}--`,
    ].join('\r\n');

    const resp = await request.post(
      apiUrl('/api/v1/admin/knowledge-bases/99999/documents/upload'),
      {
        headers: {
          ...authHeadersMultipart(token),
          'Content-Type': `multipart/form-data; boundary=${boundary}`,
        },
        data: body,
      },
    );
    await assertError(resp, 200, 10004);
  });

  test('查询文档状态 — 不存在返回 404', async ({ request }) => {
    if (!token || !kbId) { test.skip(true, '缺少 token 或知识库'); return; }
    const resp = await request.get(
      apiUrl(`/api/v1/admin/knowledge-bases/${kbId}/documents/99999/status`),
      { headers: authHeaders(token) },
    );
    await assertError(resp, 200, 10004);
  });

  test('重试文档 — 不存在返回 404', async ({ request }) => {
    if (!token || !kbId) { test.skip(true, '缺少 token 或知识库'); return; }
    const resp = await request.post(
      apiUrl(`/api/v1/admin/knowledge-bases/${kbId}/documents/99999/retry`),
      { headers: authHeaders(token) },
    );
    await assertError(resp, 200, 10004);
  });
});

// ==================== 权限验证 ====================

test.describe('权限验证', () => {
  test('无 token 访问后台知识库列表返回 401', async ({ request }) => {
    const resp = await request.get(apiUrl('/api/v1/admin/knowledge-bases'));
    await assertError(resp, 401, 10001);
  });
});

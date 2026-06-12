import { test, expect } from '@playwright/test';
import {
  requireAuth, getToken, authHeaders,
  assertSuccess, assertError, assertPaginatedResponse, assertFields,
  apiUrl, uniqueUsername, validPassword, testUserData,
} from '../utils/test-helpers.js';

/**
 * 用户管理接口集成测试 — 全覆盖 6 个端点。
 */

const token = getToken();

test.describe('GET /api/v1/admin/users', () => {
  test('返回用户列表（分页），敏感字段不泄露', async ({ request }) => {
    if (!token) { test.skip(true, '缺少 token'); return; }
    const resp = await request.get(apiUrl('/api/v1/admin/users?page=1&page_size=10'), {
      headers: authHeaders(token),
    });

    const body = await assertPaginatedResponse(resp, 1);
    const data = body.data as Array<Record<string, unknown>>;
    expect(Array.isArray(data)).toBe(true);
    if (data.length > 0) {
      const user = data[0];
      assertFields(user, {
        id: 'number', username: 'string', real_name: 'string',
        status: 'number', roles: 'array',
      });
      expect((user as Record<string, unknown>).password).toBeUndefined();
      expect((user as Record<string, unknown>).password_hash).toBeUndefined();
    }
  });

  test('无 token 访问返回 401', async ({ request }) => {
    const resp = await request.get(apiUrl('/api/v1/admin/users'));
    await assertError(resp, 401, 10001);
  });
});

test.describe('POST /api/v1/admin/users — 创建用户完整生命周期', () => {
  let userId: number;
  const userData = testUserData();

  test('创建用户成功', async ({ request }) => {
    if (!token) { test.skip(true, '缺少 token'); return; }
    const resp = await request.post(apiUrl('/api/v1/admin/users'), {
      headers: authHeaders(token),
      data: userData,
    });

    const body = await assertSuccess(resp);
    const data = body.data as Record<string, unknown>;
    expect(data.id).toBeGreaterThan(0);
    userId = data.id as number;
  });

  test('重复用户名返回 10005 冲突', async ({ request }) => {
    if (!token) { test.skip(true, '缺少 token'); return; }
    const resp = await request.post(apiUrl('/api/v1/admin/users'), {
      headers: authHeaders(token),
      data: userData, // 重复用户名
    });
    await assertError(resp, 200, 10005);
  });

  test('密码纯数字（不符合策略）返回校验失败', async ({ request }) => {
    if (!token) { test.skip(true, '缺少 token'); return; }
    const resp = await request.post(apiUrl('/api/v1/admin/users'), {
      headers: authHeaders(token),
      data: {
        username: uniqueUsername(), password: '12345678',
        real_name: '弱密码', phone: '13800001003',
      },
    });
    await assertError(resp, 200, 10003);
  });

  test('缺少必填字段返回校验失败', async ({ request }) => {
    if (!token) { test.skip(true, '缺少 token'); return; }
    const resp = await request.post(apiUrl('/api/v1/admin/users'), {
      headers: authHeaders(token),
      data: { username: uniqueUsername() },
    });
    await assertError(resp, 200, 10003);
  });

  test('查看用户详情', async ({ request }) => {
    if (!token || !userId) { test.skip(true, '缺少 token 或用户'); return; }
    const resp = await request.get(apiUrl(`/api/v1/admin/users/${userId}`), {
      headers: authHeaders(token),
    });
    const body = await assertSuccess(resp);
    const data = body.data as Record<string, unknown>;
    expect(data.id).toBe(userId);
    expect(data.username).toBe(userData.username);
  });

  test('更新用户信息', async ({ request }) => {
    if (!token || !userId) { test.skip(true, '缺少 token 或用户'); return; }
    const resp = await request.put(apiUrl(`/api/v1/admin/users/${userId}`), {
      headers: authHeaders(token),
      data: { real_name: '更新后的姓名', phone: '13800001999', email: 'updated@opsmind.local', role_ids: [4] },
    });
    await assertSuccess(resp);
  });

  test('冻结用户成功', async ({ request }) => {
    if (!token || !userId) { test.skip(true, '缺少 token 或用户'); return; }
    const resp = await request.patch(apiUrl(`/api/v1/admin/users/${userId}/freeze`), {
      headers: authHeaders(token),
    });
    await assertSuccess(resp);
  });

  test('重复冻结应失败', async ({ request }) => {
    if (!token || !userId) { test.skip(true, '缺少 token 或用户'); return; }
    const resp = await request.patch(apiUrl(`/api/v1/admin/users/${userId}/freeze`), {
      headers: authHeaders(token),
    });
    expect(resp.status()).toBe(200);
    const body = await resp.json();
    expect(body.code).toBeGreaterThan(0);
  });

  test('恢复用户成功', async ({ request }) => {
    if (!token || !userId) { test.skip(true, '缺少 token 或用户'); return; }
    const resp = await request.patch(apiUrl(`/api/v1/admin/users/${userId}/unfreeze`), {
      headers: authHeaders(token),
    });
    await assertSuccess(resp);
  });

  test('不存在的用户返回 404', async ({ request }) => {
    if (!token) { test.skip(true, '缺少 token'); return; }
    const resp = await request.get(apiUrl('/api/v1/admin/users/99999'), {
      headers: authHeaders(token),
    });
    await assertError(resp, 200, 10004);
  });
});

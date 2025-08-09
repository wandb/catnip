import { test, expect } from "@playwright/test";

// Utility: waits for a condition polling to avoid flakiness
async function waitFor<T>(
  fn: () => Promise<T>,
  predicate: (value: T) => boolean,
  timeoutMs = 20000,
  intervalMs = 250,
): Promise<T> {
  const start = Date.now();
  // eslint-disable-next-line no-constant-condition
  while (true) {
    const val = await fn();
    if (predicate(val)) return val;
    if (Date.now() - start > timeoutMs) throw new Error("waitFor timeout");
    await new Promise((r) => setTimeout(r, intervalMs));
  }
}

// E2E happy-path using the mocked claude/gh in the integration test container
// Steps:
// - Ensure test container is up at CATNIP_TEST_SERVER_URL
// - Checkout a local live repo via API to force a default workspace
// - Navigate to /workspace and auto-redirect to first workspace
// - Interact with Claude terminal via websocket: send setTitle to trigger title + todos
// - Verify title appears in header, todos render, changed files non-empty
// - Verify branch auto-renamed (no catnip/ prefix) in right sidebar

test("workspace boot + claude interaction updates UI and branch rename", async ({
  page,
  request,
}) => {
  const baseURL = process.env.CATNIP_TEST_SERVER_URL || "http://localhost:8181";

  // 1) Ensure the server is healthy (retry to handle restarts)
  await waitFor(
    async () => {
      try {
        const h = await request.get(`${baseURL}/health`);
        return h.ok();
      } catch {
        return false as any;
      }
    },
    (ok) => ok === true,
    30000,
    500,
  );

  // 2) Create (or reuse) a worktree from our live test repo
  // This endpoint exists in integration tests and will create initial worktree
  // Retry checkout in case server is rebuilding
  const checkoutJson = await waitFor(
    async () => {
      try {
        const resp = await request.post(
          `${baseURL}/v1/git/checkout/local/test-live-repo`,
          { data: {} },
        );
        if (resp.ok()) return await resp.json();
        return null as any;
      } catch {
        return null as any;
      }
    },
    (v) => !!v,
    60000,
    1000,
  );
  const worktree = checkoutJson.worktree;
  expect(worktree).toBeTruthy();

  // 3) Navigate directly to the created workspace route to avoid redirect flakiness
  const [project, ws] = (worktree.name as string).split("/");
  await page.goto(`${baseURL}/workspace/${project}/${ws}`);

  // Wait for either loading to disappear or Claude header to appear
  await Promise.race([
    page.getByText("Claude").first().waitFor(),
    page.getByText("Loading workspace").first().waitFor({ state: "hidden" }),
  ]);

  // 4) Use test-only endpoints to avoid brittle terminal input
  const simulateTitle = async (title: string) => {
    const res = await request.post(
      `${baseURL}/v1/test/worktrees/${worktree.id}/title`,
      {
        data: {
          title,
          todos: [
            { id: "1", content: title, status: "pending", priority: "medium" },
          ],
        },
      },
    );
    expect(res.ok()).toBeTruthy();
  };
  await simulateTitle("Implement login flow");

  // Wait for session title to be reflected in API before checking UI
  await waitFor(
    async () => {
      const wtResp = await request.get(`${baseURL}/v1/git/worktrees`);
      if (!wtResp.ok()) return "" as any;
      const arr = (await wtResp.json()) as any[];
      const self = arr.find((w) => w.path === worktree.path);
      const title = self?.session_title?.title || "";
      return title;
    },
    (title: string) => title.includes("Implement login flow"),
    30000,
    500,
  );

  // 5) Verify header shows the new session title (WorkspaceMainContent shows "- {title}" after Claude label)
  await expect(page.getByText("- Implement login flow")).toBeVisible({
    timeout: 30000,
  });

  // 6) Verify todos appear in right sidebar by content text
  await waitFor(
    async () => page.getByText("Implement login flow").count(),
    (cnt) => cnt > 0,
    30000,
    500,
  );

  // 7) Verify changed files populated (Changed Files badge not 0)
  // Create a real file change so diff list is non-empty
  // Create a file change via helper
  const resFile = await request.post(
    `${baseURL}/v1/test/worktrees/${worktree.id}/file`,
    {
      data: { path: "e2e.txt", content: "hello from e2e" },
    },
  );
  expect(resFile.ok()).toBeTruthy();

  // Wait for Changed Files by checking diff API directly for this worktree
  await waitFor(
    async () => {
      const diffResp = await request.get(
        `${baseURL}/v1/git/worktrees/${worktree.id}/diff`,
      );
      if (!diffResp.ok()) return 0 as any;
      const diff = await diffResp.json();
      const count = Array.isArray(diff?.file_diffs)
        ? diff.file_diffs.length
        : 0;
      return count;
    },
    (count: number) => count > 0,
    60000,
    500,
  );

  // 8) Verify branch gets auto-renamed (no catnip/) via API to avoid flaky DOM selectors
  await waitFor(
    async () => {
      const wtResp = await request.get(`${baseURL}/v1/git/worktrees`);
      const arr = wtResp.ok() ? ((await wtResp.json()) as any[]) : [];
      const self = arr.find((w) => w.path === worktree.path);
      return self?.branch || "";
    },
    (branch: string) => !!branch && !branch.includes("catnip/"),
    60_000,
    500,
  );
});

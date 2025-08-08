import { renderHook, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { useGitActions } from "../useGitActions";

// Mock the git API
vi.mock("../../lib/git-api", () => ({
  gitApi: {
    checkoutRepository: vi.fn(),
    getStatus: vi.fn(),
    listWorktrees: vi.fn(),
  },
}));

// Mock the app store
const mockSetError = vi.fn();
const mockSetLoading = vi.fn();
const mockSetGitStatus = vi.fn();

vi.mock("../../stores/appStore", () => ({
  useAppStore: vi.fn((selector) => {
    const state = {
      setError: mockSetError,
      setLoading: mockSetLoading,
      setGitStatus: mockSetGitStatus,
      gitStatus: null,
      error: null,
      isLoading: false,
    };
    return selector(state);
  }),
}));

describe("useGitActions", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should initialize with correct default values", () => {
    const { result } = renderHook(() => useGitActions());

    expect(result.current).toHaveProperty("checkoutRepository");
    expect(result.current).toHaveProperty("refreshStatus");
    expect(result.current).toHaveProperty("createWorktree");
    expect(typeof result.current.checkoutRepository).toBe("function");
    expect(typeof result.current.refreshStatus).toBe("function");
    expect(typeof result.current.createWorktree).toBe("function");
  });

  it("should handle checkout repository", async () => {
    const { gitApi } = await import("../../lib/git-api");
    const mockCheckout = gitApi.checkoutRepository as any;

    mockCheckout.mockResolvedValueOnce({
      repository: { id: "test/repo" },
      worktree: { id: "wt-123" },
    });

    const { result } = renderHook(() => useGitActions());

    const checkoutPromise = result.current.checkoutRepository(
      "test-org",
      "test-repo",
    );

    expect(mockSetLoading).toHaveBeenCalledWith(true);

    await waitFor(() => {
      expect(checkoutPromise).resolves.toBeDefined();
    });

    expect(mockCheckout).toHaveBeenCalledWith(
      "test-org",
      "test-repo",
      undefined,
    );
  });

  it("should handle errors during checkout", async () => {
    const { gitApi } = await import("../../lib/git-api");
    const mockCheckout = gitApi.checkoutRepository as any;

    mockCheckout.mockRejectedValueOnce(new Error("Checkout failed"));

    const { result } = renderHook(() => useGitActions());

    await expect(
      result.current.checkoutRepository("test-org", "bad-repo"),
    ).rejects.toThrow("Checkout failed");

    expect(mockSetError).toHaveBeenCalledWith("Checkout failed");
    expect(mockSetLoading).toHaveBeenCalledWith(false);
  });

  it("should refresh git status", async () => {
    const { gitApi } = await import("../../lib/git-api");
    const mockGetStatus = gitApi.getStatus as any;

    const mockStatus = {
      repositories: { "test/repo": { id: "test/repo" } },
      worktreeCount: 1,
    };
    mockGetStatus.mockResolvedValueOnce(mockStatus);

    const { result } = renderHook(() => useGitActions());

    await result.current.refreshStatus();

    expect(mockGetStatus).toHaveBeenCalled();
    expect(mockSetGitStatus).toHaveBeenCalledWith(mockStatus);
  });

  it("should handle status refresh errors", async () => {
    const { gitApi } = await import("../../lib/git-api");
    const mockGetStatus = gitApi.getStatus as any;

    mockGetStatus.mockRejectedValueOnce(new Error("Status failed"));

    const { result } = renderHook(() => useGitActions());

    await expect(result.current.refreshStatus()).rejects.toThrow(
      "Status failed",
    );

    expect(mockSetError).toHaveBeenCalledWith("Status failed");
  });
});

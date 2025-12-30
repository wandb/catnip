import { SignJWT, importPKCS8 } from "jose";

export interface ApnsConfig {
  authKey: string;
  keyId: string;
  teamId: string;
  bundleId: string;
}

export interface ApnsAlert {
  title: string;
  body: string;
}

export interface ApnsPayload {
  aps: {
    alert: ApnsAlert;
    sound?: string;
    badge?: number;
    "mutable-content"?: number;
    "thread-id"?: string;
  };
  workspaceId?: string;
  workspaceName?: string;
  action?: string;
}

// Live Activity content state - matches iOS CodespaceActivityAttributes.ContentState
export interface LiveActivityContentState {
  status: string;
  progress: number;
  elapsedSeconds: number;
}

export interface LiveActivityUpdateOptions {
  staleDate?: number; // Unix timestamp when the data becomes stale
  dismissalDate?: number; // Unix timestamp when to dismiss the activity
  event?: "update" | "end"; // 'end' to dismiss the activity
}

export async function sendPushNotification(
  deviceToken: string,
  payload: ApnsPayload,
  config: ApnsConfig,
): Promise<{ success: boolean; error?: string }> {
  try {
    // Generate JWT for APNs authentication
    const privateKey = await importPKCS8(config.authKey, "ES256");

    const jwt = await new SignJWT({})
      .setProtectedHeader({ alg: "ES256", kid: config.keyId })
      .setIssuer(config.teamId)
      .setIssuedAt()
      .setExpirationTime("1h")
      .sign(privateKey);

    // APNs production endpoint (HTTP/2)
    const url = `https://api.push.apple.com/3/device/${deviceToken}`;

    const response = await fetch(url, {
      method: "POST",
      headers: {
        authorization: `bearer ${jwt}`,
        "apns-topic": config.bundleId,
        "apns-push-type": "alert",
        "apns-priority": "10",
      },
      body: JSON.stringify(payload),
    });

    if (!response.ok) {
      const error = (await response.json()) as { reason?: string };
      console.error("APNs error:", response.status, error);
      return {
        success: false,
        error: error.reason || `HTTP ${response.status}`,
      };
    }

    console.log(
      `✅ Push notification sent to device ${deviceToken.slice(0, 8)}...`,
    );
    return { success: true };
  } catch (error) {
    console.error("APNs send error:", error);
    return { success: false, error: String(error) };
  }
}

/**
 * Send a Live Activity push notification to update the codespace creation progress widget.
 * Uses a different APNs topic and payload format than regular push notifications.
 */
export async function sendLiveActivityUpdate(
  pushToken: string,
  contentState: LiveActivityContentState,
  config: ApnsConfig,
  options?: LiveActivityUpdateOptions,
): Promise<{ success: boolean; error?: string }> {
  try {
    // Generate JWT for APNs authentication
    const privateKey = await importPKCS8(config.authKey, "ES256");

    const jwt = await new SignJWT({})
      .setProtectedHeader({ alg: "ES256", kid: config.keyId })
      .setIssuer(config.teamId)
      .setIssuedAt()
      .setExpirationTime("1h")
      .sign(privateKey);

    // APNs production endpoint (HTTP/2)
    const url = `https://api.push.apple.com/3/device/${pushToken}`;

    // Build Live Activity payload
    const payload: {
      aps: {
        timestamp: number;
        event: "update" | "end";
        "content-state": LiveActivityContentState;
        "stale-date"?: number;
        "dismissal-date"?: number;
      };
    } = {
      aps: {
        timestamp: Math.floor(Date.now() / 1000),
        event: options?.event || "update",
        "content-state": contentState,
      },
    };

    // Add optional stale-date (defaults to 60 seconds from now for regular updates)
    if (options?.staleDate) {
      payload.aps["stale-date"] = options.staleDate;
    } else if (options?.event !== "end") {
      // Default stale date: 60 seconds from now
      payload.aps["stale-date"] = Math.floor(Date.now() / 1000) + 60;
    }

    // Add dismissal-date for end events
    if (options?.dismissalDate) {
      payload.aps["dismissal-date"] = options.dismissalDate;
    } else if (options?.event === "end") {
      // Default dismissal: 3 seconds from now (brief final display)
      payload.aps["dismissal-date"] = Math.floor(Date.now() / 1000) + 3;
    }

    // Live Activity uses different topic format
    const liveActivityTopic = `${config.bundleId}.push-type.liveactivity`;

    const response = await fetch(url, {
      method: "POST",
      headers: {
        authorization: `bearer ${jwt}`,
        "apns-topic": liveActivityTopic,
        "apns-push-type": "liveactivity",
        "apns-priority": "10",
      },
      body: JSON.stringify(payload),
    });

    if (!response.ok) {
      const error = (await response.json()) as { reason?: string };
      console.error("APNs Live Activity error:", response.status, error);
      return {
        success: false,
        error: error.reason || `HTTP ${response.status}`,
      };
    }

    console.log(
      `✅ Live Activity update sent: ${contentState.status} (${Math.round(contentState.progress * 100)}%)`,
    );
    return { success: true };
  } catch (error) {
    console.error("APNs Live Activity send error:", error);
    return { success: false, error: String(error) };
  }
}

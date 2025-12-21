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

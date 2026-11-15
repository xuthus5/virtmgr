import type { GuestAgentAction, GuestAgentInfo, VMState } from "@/api/types";

export function guestAgentActionAvailable(info: GuestAgentInfo, action: GuestAgentAction) {
  switch (action) {
    case "shutdown": return info.capabilities.shutdown;
    case "reboot": return info.capabilities.reboot;
    case "sync-time": return info.capabilities.timeSync;
    case "trim": return info.capabilities.trim;
  }
}

export function guestAgentActionUnavailableReason(info: GuestAgentInfo, action: GuestAgentAction) {
  if (info.state !== "connected") return "QEMU Guest Agent 未连接";
  if (!guestAgentActionAvailable(info, action)) return "当前代理版本不支持该能力";
  return "";
}

export function guestAgentCanAddChannel(state: VMState) {
  return state === "shutoff" || state === "crashed";
}


/**
 * Thin MCP JSON-RPC helpers for Context Fabric tools/call.
 * Skills/orchestration layers must never treat these helpers as an AuthZ boundary.
 */

import type { ContextPacket } from "./client.js";

/** ContextPacket schema version (independent of API version). */
export const PACKET_VERSION = "context-fabric.packet/v1";

export type JSONRPCRequest = {
  jsonrpc: "2.0";
  id: string | number;
  method: string;
  params?: unknown;
};

export type MCPToolCallParams = {
  name: string;
  arguments?: Record<string, unknown>;
};

export type MCPContentBlock = {
  type: string;
  text?: string;
  [key: string]: unknown;
};

export type MCPToolResult = {
  content?: MCPContentBlock[];
  isError?: boolean;
  [key: string]: unknown;
};

/** Build a JSON-RPC tools/call request body. */
export function buildToolsCall(
  id: string | number,
  name: string,
  args: Record<string, unknown> = {},
): JSONRPCRequest {
  const params: MCPToolCallParams = { name, arguments: args };
  return {
    jsonrpc: "2.0",
    id,
    method: "tools/call",
    params,
  };
}

/**
 * Parse a ContextPacket from an MCP tools/call result.
 * Expects content[0].text to be JSON (as returned by context-fabric MCP).
 */
export function parseContextPacketFromToolResult(
  result: MCPToolResult,
): ContextPacket {
  if (!result?.content?.length) {
    throw new Error("MCP tool result has no content");
  }
  const block = result.content[0];
  if (block.type !== "text" || typeof block.text !== "string") {
    throw new Error("MCP tool result content[0] is not text");
  }
  const parsed = JSON.parse(block.text) as ContextPacket;
  return parsed;
}

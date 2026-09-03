#!/usr/bin/env python3
"""
MCP 2.x test server for E2E testing with Go SDK.

This server implements:
- tools/list and tools/call with Output Schema
- resources/list with Resource Templates
- resources/read
- prompts/list and prompts/get
- Version negotiation support
"""

import json
import sys
from mcp.server import Server
from mcp.server.stdio import stdio_server
from mcp.types import (
    Tool,
    Resource,
    ResourceTemplate,
    Prompt,
    PromptArgument,
    TextContent,
    CallToolResult,
    ReadResourceResult,
    TextResourceContents,
    GetPromptResult,
    PromptMessage,
)


async def main():
    server = Server("test-mcp-2x-server")

    @server.list_tools()
    async def list_tools():
        return [
            Tool(
                name="greet",
                description="Greet a user by name",
                inputSchema={
                    "type": "object",
                    "properties": {
                        "name": {"type": "string", "description": "Name to greet"},
                    },
                    "required": ["name"],
                },
                outputSchema={
                    "type": "object",
                    "properties": {
                        "greeting": {"type": "string"},
                        "timestamp": {"type": "string"},
                    },
                },
            ),
            Tool(
                name="calculate",
                description="Perform a calculation",
                inputSchema={
                    "type": "object",
                    "properties": {
                        "expression": {"type": "string", "description": "Math expression"},
                    },
                    "required": ["expression"],
                },
            ),
        ]

    @server.call_tool()
    async def call_tool(name: str, arguments: dict):
        if name == "greet":
            user_name = arguments.get("name", "World")
            return CallToolResult(
                content=[TextContent(type="text", text=f"Hello, {user_name}!")],
                structuredContent={
                    "greeting": f"Hello, {user_name}!",
                    "timestamp": "2025-01-01T00:00:00Z",
                },
            )
        elif name == "calculate":
            expr = arguments.get("expression", "0")
            try:
                result = eval(expr)  # noqa: S307
                return CallToolResult(
                    content=[TextContent(type="text", text=f"{expr} = {result}")],
                    structuredContent={"expression": expr, "result": result},
                )
            except Exception as e:
                return CallToolResult(
                    content=[TextContent(type="text", text=f"Error: {e}")],
                    isError=True,
                )
        else:
            return CallToolResult(
                content=[TextContent(type="text", text=f"Unknown tool: {name}")],
                isError=True,
            )

    @server.list_resources()
    async def list_resources():
        return [
            Resource(
                uri="test://hello",
                name="Hello World",
                description="A simple test resource",
                mimeType="text/plain",
            ),
        ]

    @server.read_resource()
    async def read_resource(uri: str):
        if uri == "test://hello":
            return ReadResourceResult(
                contents=[
                    TextResourceContents(
                        uri="test://hello",
                        text="Hello, World!",
                        mimeType="text/plain",
                    )
                ]
            )
        raise ValueError(f"Unknown resource: {uri}")

    @server.list_resource_templates()
    async def list_resource_templates():
        return [
            ResourceTemplate(
                uriTemplate="test://{name}",
                name="Test Resources",
                description="Access test resources by name",
                mimeType="text/plain",
            ),
        ]

    @server.list_prompts()
    async def list_prompts():
        return [
            Prompt(
                name="greeting",
                description="Generate a greeting message",
                arguments=[
                    PromptArgument(
                        name="name",
                        description="Name to greet",
                        required=True,
                    ),
                ],
            ),
        ]

    @server.get_prompt()
    async def get_prompt(name: str, arguments: dict):
        if name == "greeting":
            user_name = arguments.get("name", "World")
            return GetPromptResult(
                description=f"Greeting for {user_name}",
                messages=[
                    PromptMessage(
                        role="user",
                        content=TextContent(
                            type="text",
                            text=f"Please greet {user_name}",
                        ),
                    ),
                ],
            )
        raise ValueError(f"Unknown prompt: {name}")

    async with stdio_server() as (read_stream, write_stream):
        await server.run(
            read_stream,
            write_stream,
            server.create_initialization_options(),
        )


if __name__ == "__main__":
    import asyncio
    asyncio.run(main())

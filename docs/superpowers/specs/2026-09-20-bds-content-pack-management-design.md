# BDS Content Pack Management and Server Controls

## Purpose

Extend the existing NetherNet Multi-Server Administrator with server-side Bedrock add-on management and background server controls. The existing LAN discovery, connection proxy, Microsoft account pool, player assignment, server advertisements, and world data behavior must remain unchanged.

The operator downloads add-on files separately. The administrator imports and manages those local files for the Creative and Survival BDS instances.

## Existing Deployment

- Creative BDS root: `C:\MinecraftServers\Creative`
- Creative world: `C:\MinecraftServers\Creative\worlds\Private Creative Server`
- Survival BDS root: `C:\MinecraftServers\Survival`
- Survival world: `C:\MinecraftServers\Survival\worlds\Private Survival Server`
- Administrator/helper root: `C:\MinecraftServers\MultiServer`
- Creative and Survival continue using their existing addresses, ports, worlds, and NetherNet settings.

## Architecture

The background `NetherNetMultiServer.exe` process becomes the supervisor for both BDS processes. The GUI remains a separate client of the existing local administration channel.

The supervisor:

- starts each configured `bedrock_server.exe` with its BDS directory as the working directory;
- creates the BDS process without a visible console window;
- retains standard input so it can issue the BDS `stop` command;
- captures standard output and standard error into bounded in-memory recent-log buffers and persistent log files;
- exposes server status and lifecycle operations through the local admin API;
- owns pending pack operations and applies them only while the corresponding BDS is stopped.

Closing the GUI never stops BDS. Stopping the helper does not intentionally terminate BDS, although graceful control is unavailable until the helper is restarted and takes ownership of a newly launched BDS process.

An already-running BDS process that was not launched by the helper is reported as `Externally running`. It can be monitored using process and port information, but the GUI does not force-kill it or claim graceful control. The operator stops it using its existing mechanism once; subsequent starts through the GUI are fully managed.

## Configuration

Each existing server entry gains a `process` section:

```json
{
  "executable": "C:\\MinecraftServers\\Creative\\bedrock_server.exe",
  "working_directory": "C:\\MinecraftServers\\Creative",
  "world_directory": "C:\\MinecraftServers\\Creative\\worlds\\Private Creative Server",
  "log_directory": "C:\\MinecraftServers\\Creative\\logs"
}
```

The Survival entry uses the equivalent Survival paths. Configuration validation requires absolute paths, ensures the executable and working directory agree, and ensures the world directory is contained beneath the configured BDS root. Existing configurations without `process` sections continue to run the current NetherNet/account functionality, but server and pack controls are shown as unconfigured.

## Server Lifecycle

Each server has the following states:

- `Stopped`
- `Starting`
- `Running`
- `Stopping`
- `Externally running`
- `Failed`
- `Unconfigured`

### Start

Start is available only for a configured, stopped server. Before launch, the supervisor applies all validated pending pack changes for that server. If pack application fails, BDS is not started and the error is shown in the GUI.

### Stop

For a managed BDS, the supervisor writes `stop` to standard input and waits for a configurable grace period. It never force-terminates the process automatically. A timeout is reported and requires an explicit operator decision outside this initial scope.

If players are connected, the GUI requires confirmation and states how many sessions may be disconnected.

### Restart

Restart performs the same confirmed graceful stop followed by pending-pack application and start. The button is labelled `Restart`; no action in this feature is called `Reset`, preventing confusion with world deletion.

### Status and Logs

The supervisor reports state, process ID, start time, uptime, exit code when applicable, connected players known to the NetherNet proxy, and recent BDS output. Persistent log files use timestamped names and append no authentication tokens.

Concurrent lifecycle requests for the same server are serialized. Duplicate starts and stops return clear state-specific errors.

## Pack Import

The GUI accepts local `.mcaddon`, `.mcpack`, and `.zip` files through a file picker. Import does not download content from the internet.

The importer treats all archives as untrusted input and:

- enforces a maximum compressed and expanded size;
- limits file count and nesting depth;
- rejects absolute paths, drive-qualified paths, path traversal, symlinks, and files outside the staging directory;
- locates and parses every `manifest.json`;
- validates manifest format, header UUID, version, module type, minimum engine version, and dependencies;
- identifies behavior packs and resource packs;
- groups linked packs using manifest dependencies;
- detects UUID/version conflicts with installed and staged packs;
- reports incompatible or ambiguous archives without modifying either server.

Valid imports are copied into a helper-owned staging area under `C:\MinecraftServers\MultiServer\content-staging`. Imported source files are never executed.

## Pack Assignment and Activation

The Content Packs tab shows an inventory of imported packs and their assignment to Creative, Survival, or both.

For each server, an imported pack can be:

- enabled;
- disabled;
- updated by a higher version with the same pack UUID;
- uninstalled;
- assigned or unassigned.

Linked behavior/resource packs are presented as one add-on where possible and changed together. The GUI displays pack name, description, version, UUID, type, minimum engine version, dependencies, installation state, and whether a restart is pending.

Activation is world-specific. The supervisor maintains the selected world’s `world_behavior_packs.json` and `world_resource_packs.json`, using the UUID and version from each validated manifest. Pack files are installed into the selected BDS root’s `behavior_packs` and `resource_packs` directories.

`texturepack-required` remains a per-server option shown as `Require clients to accept resource packs`. Changing it is staged and updates `server.properties` only during pack application.

## Staging and Transaction Safety

Pack changes requested while BDS is running are recorded as pending and do not touch active server or world files. Changes requested while stopped are still staged first and then applied as one transaction.

Before applying a transaction, the supervisor creates a timestamped backup containing:

- `world_behavior_packs.json`, if present;
- `world_resource_packs.json`, if present;
- `server.properties` when its resource-pack setting changes;
- the pack directories affected by update or removal;
- a machine-readable operation manifest.

Application writes temporary files and renames them into place. If any step fails, the supervisor restores the backed-up activation and configuration files and leaves a clear failure report. Backups are retained until manually removed; automatic retention policy is outside the initial scope.

No operation deletes or rewrites the world database, `level.dat`, player data, NetherNet identities, or Microsoft account data.

## Administrator UI

The existing account and console-user interface remains available. The main window adds:

### Servers Tab

One card per configured server with:

- server name and current state;
- connected-player count;
- PID and uptime;
- green `Start` button;
- red `Stop` button;
- amber `Restart` button;
- recent BDS console output;
- pending-change indicator.

Buttons are enabled only when valid for the current state. Stop and Restart show a confirmation dialog when players are connected.

### Content Packs Tab

- `Import pack` file picker;
- installed/imported pack list;
- Creative and Survival assignment and enablement state;
- pack details and validation problems;
- `Enable`, `Disable`, `Update`, and `Uninstall` actions;
- `Require clients to accept resource packs` per-server setting;
- pending-operation summary;
- `Apply now` when the selected server is stopped;
- explanation that otherwise changes apply at the next restart.

Color is supplementary to text and state icons; state must remain understandable without color.

## Administration API

The local admin API gains operations for:

- server status and recent logs;
- server start, stop, and restart;
- pack inventory and detailed validation results;
- local archive import;
- staged assignment, enable, disable, update, uninstall, and resource-pack policy changes;
- pending-operation inspection and cancellation;
- explicit application while stopped.

The helper, not the GUI, performs filesystem and process operations. Requests identify servers using stable configured IDs rather than display names or table positions.

## Error Handling

All failures identify the affected server and operation. Expected errors include invalid archive, unsafe path, malformed manifest, incompatible engine version, missing dependency, UUID/version conflict, BDS externally running, invalid lifecycle state, graceful-stop timeout, backup failure, apply failure, and rollback failure.

Ordinary local details remain visible for this single-operator deployment. Authentication tokens and bearer credentials continue to be filtered from admin responses and logs.

## Verification

Automated tests cover:

- archive traversal, expansion, count, and size protections;
- manifest discovery and validation;
- behavior/resource dependency grouping;
- conflict and version handling;
- generated world activation JSON;
- staged operation persistence;
- backup, atomic application, and rollback;
- server lifecycle state transitions and duplicate-operation rejection;
- graceful stop command delivery using a controlled fake process;
- admin API request/response behavior;
- GUI view-model state and action enablement;
- compatibility with configurations that omit process management.

Manual Windows verification covers:

1. Import a known-compatible behavior/resource add-on.
2. Assign it to Creative only while Creative is running.
3. Confirm the server remains running and the change shows as pending.
4. Restart Creative from the GUI and confirm a graceful stop, pack application, hidden restart, and successful world load.
5. Join Creative from PS5 and Switch 2 and verify the add-on/resource pack.
6. Confirm Survival is unchanged.
7. Disable the add-on, restart Creative, and verify removal from the active world.
8. Exercise a deliberately invalid archive and confirm no server or world files change.
9. Confirm the existing two-server discovery, connection proxy, account pool, and player reassignment still work.

## Out of Scope

- Downloading or browsing add-ons from external websites.
- Java Edition mods, plugins, Forge, Fabric, Bukkit, or Paper.
- Editing an add-on’s internal gameplay settings.
- Automatic forced process termination.
- World deletion, reset, replacement, or rollback.
- Remote administration or multi-user authentication for the GUI.
- Automatic backup pruning.

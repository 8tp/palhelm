# Palhelm native Pal bridge

Experimental, server-only Linux UE4SS C++ provider for Palhelm's Give Pal
workflow. It is not installed or production-ready.

## Retired exact target

- Palworld executable build ID: `7f7e167407984ec3`
- Palworld executable SHA-256:
  `788649fa1592160faa7bcf07ccd16d474ebeaae954717bc32284b5a43028d8e7`
- UE4SS fork: `Yangff/RE-UE4SS`, `linux-port-rebase2`
- UE4SS commit: `9b4552068804a5f5ec2309ca378aeb65af40ee1a`
- Installed `libUE4SS.so` SHA-256:
  `b614f4f0555319be60a45fd5c47e7ca8089a3222babf49a42d4bdbd187855fb1`

This Palworld target is no longer installed. As of 2026-08-02, the live server
runs `v1.0.2.101103` / Steam build `24466863`, with GNU build ID
`787f7f8c15edb8fb` and executable SHA-256
`c508a28b06cebf0752296b38da5244c08a5688da44dad8f816eb2d726d82699e`.
The probe must remain disabled against it. Do not update the constants below
until a fresh UHT dump, `MemberVariableLayout.ini`, and reflected metadata audit
have been completed on a disposable server for the exact new target.

The installed fork loads `Mods/<mod>/dlls/main.so` with `dlopen` and resolves
`start_mod` plus `uninstall_mod`. This is not upstream's stable Linux C++ ABI.
The minimal ABI mirror is therefore pinned to the exact fork commit and includes
the `HAS_UI` base-class member and virtual slots used by that build.

## Current capability probe

The first artifact is deliberately non-mutating. On Unreal initialization it:

1. reads the GNU build ID from the running main executable;
2. refuses to continue on any build other than the retired pinned Palworld build;
3. checks that the four reflected functions required by the proposed native
   spawn-and-capture path still exist;
4. logs readiness, but does not read a request or create a Pal.

The bridge links to the already-loaded `libUE4SS.so` without linking a second
C++ standard library. That avoids the static-runtime symbol interposition called
out by current upstream Linux experiments.

## Build

Run `./mods/palhelm-pal-native/build.sh`. The disposable builder uses Clang and
libc++ because the installed UE4SS exports the `std::__1` ABI. The output is
`dist/main.so`.

Before any live install, inspect the artifact with `readelf`, verify its only
UE4SS dependency is `libUE4SS.so`, hash both live target binaries again, take and
verify a world backup, and obtain explicit restart approval.

`./mods/palhelm-pal-native/verify-target.sh` performs the target hash, layout,
and reflected-header preflight. A pass authorizes staging only; it does not
authorize installing, loading, restarting, or mutating the world.

## Planned mutation provider

The first grant implementation will remain behind Palhelm's existing exact-one-
request validation gate:

1. stage one request off-thread without touching Unreal objects;
2. execute state transitions from a native ProcessEvent pre-hook on the game
   thread;
3. resolve the target by stable player UID and preflight party/Palbox capacity;
4. allocate and initialize reflected parameter buffers through `FProperty`;
5. call `GetInitializedCharacterSaveParemter` natively so the large out struct is
   never marshalled through Lua;
6. create/spawn one individual, then deliver through Palworld's authoritative
   capture/grant path;
7. verify owner UID, container, slot, instance ID, and generated fields;
8. refuse the mutation unless a current-build, authoritative cleanup route is
   proven; the stale native symbols cannot be used as rollback;
9. report any timeout after dispatch as `outcome_unknown`, with no retry.

The native-only cleanup and direct-handle grant helper names exist in the
installed Palworld `.sym` bundle, but that bundle predates the current executable
and its records do not land on the named functions in the current ELF. It is
therefore rejected as stale. No address from it may be called. Until Pocketpair
ships matching symbols or the helpers can be reached through a validated reflected
API, the capability state must remain `validation_required`.

`tools/resolve_sym.cpp` implements Unreal 5.6's compact Breakpad symbol-record
format for offline, exact-name inspection. Its output demonstrated the mismatch:
the July 9 symbol records do not correspond to the July 15 executable's function
boundaries. It is a reverse-engineering aid only, never a runtime resolver.

# Per-channel cooldown override

Root administrators can set the `ChannelCooldownDisabled` option to a JSON object
mapping channel IDs to booleans. The default is `{}`. For example, setting
`{"149":true}` opts channel 149 out of cooldown; all other channels retain the
normal policy. This is independent of the channel's automatic-disable switch.

Use `PUT /api/option/` with the normal root-administrator authentication:

```json
{"key":"ChannelCooldownDisabled","value":"{\"149\":true}"}
```

The update takes effect immediately and persists across deployments. While enabled,
existing local/Redis cooldowns are ignored, new channel cooldowns are not stored,
and adaptive or shared-host circuits cannot exclude that channel from selection.
Health observations and pricing remain unchanged. An override does not enable a
manually or automatically disabled channel, waive upstream errors or balance checks,
or bypass same-request channel exclusions and retry limits.

Set its value to `false`, or remove its entry, to restore the normal policy.
An unexpired cooldown recorded before the override can then apply again.

For the Wan 3.0 installation, only standard Aijiau channel 149 is opted out.
Prime channel 150 retains its existing policy, and retired channel 147 remains
disabled. Requests exceeding the provider account balance can still fail; this
setting prevents those failures from putting channel 149 into a cooldown period.

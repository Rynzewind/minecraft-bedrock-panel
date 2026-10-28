# Flexible automatic snapshot schedule and migration instructions

Replace the old installer-folder message with single-file NetherNetSetup.exe + snapshot ZIP instructions. Add an enabled checkbox, 1–7 day dropdown (daily, every 2–6 days, weekly), hour/minute dropdowns, and a weekday dropdown for weekly (Monday default). Save schedule applies choices to the existing owned Windows task. Use Windows daily interval triggers for 1–6 days and weekly trigger for 7; native task state preserves the interval anchor without forcing an extra snapshot. First run uses the next chosen local date/time; weekly keeps chosen weekday. Display last attempted run, result and next run, with successful archive paths still in snapshot-schedule.log. Continue full addon inclusion, missed-run catch-up and no overlaps.

Migrate old weekly JSON to enabled/7 days/Monday/00:00. Preserve task name and ownership guards. Editing a folder preserves stored frequency/time. UI settings are saved so no rebuild is needed for further changes.

Test schedule defaults/migration/validation and next trigger across day boundaries; mocked native task specs for daily, 3-day and weekly; migration summary message; panel tests/vet. Update installed admin/control only without restarting Minecraft, scan/package new release and publish current single EXE to Drive through release wrapper.

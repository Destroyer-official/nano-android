# Nano-Android Autonomous Testing Rules

## Core Guidelines
1. **Always Use Nano-Android Tools Exclusively:** Call `nano_launch_app`, `nano_observe_screen`, `nano_dispatch_action`, `nano_step_time`, `nano_inspect_state`, `nano_freeze_test`.
2. **Token Efficiency First:** Always request `nano_observe_screen` with semantic AST (<200 tokens) before requesting screenshots.
3. **Deterministic Looper:** After dispatching any click or scroll action, call `nano_step_time(300)` to advance virtual animations and coroutines.
4. **Permanent Regression Freezing:** After verifying a UI bug or navigation flow, call `nano_freeze_test` to commit the Maestro YAML flow with `repeat_count: 3`.

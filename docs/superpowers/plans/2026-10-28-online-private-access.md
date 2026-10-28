# Secure Online preparation implementation plan
1. Inspect existing identities/auth/lifecycle and pinned broadcasters. Record blockers before networking changes.
2. TDD account roles/migration, strict default-deny persisted policy and isolated capability worker (parallel independent packages).
3. Integrate token-free service APIs, profile-confirmation grants, Accounts roles and Online four subtabs. Existing Servers remains master lifecycle.
4. Add migration/settings/packaging support with candidate-only build, no live installer publication. Run packages/native/full tests and isolated module tests.
5. Review safety, document every implemented/incomplete/unverified acceptance requirement and rollback; deliver local candidate artifacts only.

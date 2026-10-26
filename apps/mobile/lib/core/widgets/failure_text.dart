import '../api/api_failure.dart';

/// What to tell the player about a failure, in plain words. The server's own title and detail are used when it gave
/// them, because it knows the reason best.
({String title, String? detail}) describeFailure(ApiFailure f) => switch (f) {
  ProblemFailure(:final title, :final detail, :final kind) => (
    title: title,
    detail:
        detail ??
        switch (kind) {
          FailureKind.rateLimited => 'Please wait a moment and try again.',
          FailureKind.server => 'The service is busy or unavailable. Please try again shortly.',
          _ => null,
        },
  ),
  NetworkFailure(:final timedOut) => (
    title: timedOut ? 'The server took too long to answer' : "You're offline or the connection dropped",
    detail: 'Check your connection and try again.',
  ),
  MalformedResponseFailure() => (title: 'Unexpected answer from the server', detail: 'Please try again shortly.'),
  SessionEndedFailure() => (title: 'Your session has ended', detail: 'Sign in again to continue.'),
};

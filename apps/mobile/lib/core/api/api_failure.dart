/// Why a request did not give a usable answer. Screens explain each kind differently; writes look at
/// [outcomeUncertain] before ever saying something failed.
enum FailureKind {
  validation,
  authentication,
  eligibility,
  notFound,
  conflict,
  rateLimited,
  server,
  network,
  malformed,
}

sealed class ApiFailure implements Exception {
  const ApiFailure();

  FailureKind get kind;

  /// True when a write may have taken effect although no clear answer arrived: reconcile, never assume either way.
  bool get outcomeUncertain;
}

/// An RFC 7807 problem the API answered with.
final class ProblemFailure extends ApiFailure {
  const ProblemFailure({required this.status, required this.slug, required this.title, this.detail, this.retryAfter});

  final int status;

  /// The last segment of the problem `type`, e.g. `daily-limit`.
  final String slug;
  final String title;
  final String? detail;
  final Duration? retryAfter;

  @override
  FailureKind get kind => switch (status) {
    400 || 422 => FailureKind.validation,
    401 => FailureKind.authentication,
    403 => FailureKind.eligibility,
    404 => FailureKind.notFound,
    409 => FailureKind.conflict,
    429 => FailureKind.rateLimited,
    _ when status >= 500 => FailureKind.server,
    _ => FailureKind.validation,
  };

  /// A 5xx may come from a proxy after the API committed; `request-in-progress` means the first attempt still runs.
  @override
  bool get outcomeUncertain => status >= 500 || slug == 'request-in-progress';

  @override
  String toString() => 'ProblemFailure($status $slug: $title)';
}

/// No answer: offline, connection reset, DNS, or [timedOut].
final class NetworkFailure extends ApiFailure {
  const NetworkFailure({this.timedOut = false, this.cause});

  final bool timedOut;
  final Object? cause;

  @override
  FailureKind get kind => FailureKind.network;

  @override
  bool get outcomeUncertain => true;

  @override
  String toString() => 'NetworkFailure(timedOut: $timedOut)';
}

/// An answer that could not be read. A 2xx one may mean the write happened.
final class MalformedResponseFailure extends ApiFailure {
  const MalformedResponseFailure(this.status, this.reason);

  final int status;
  final String reason;

  @override
  FailureKind get kind => FailureKind.malformed;

  @override
  bool get outcomeUncertain => status < 300 || status >= 500;

  @override
  String toString() => 'MalformedResponseFailure($status: $reason)';
}

/// The session is over (signed out elsewhere, expired, revoked). The request was refused before it ran.
final class SessionEndedFailure extends ApiFailure {
  const SessionEndedFailure();

  @override
  FailureKind get kind => FailureKind.authentication;

  @override
  bool get outcomeUncertain => false;
}

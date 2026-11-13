import '../../../core/api/json.dart';

enum KycStatus { none, pending, verified, manualReview, rejected }

KycStatus kycStatusFrom(String s) => switch (s) {
  'none' => KycStatus.none,
  'pending' => KycStatus.pending,
  'verified' => KycStatus.verified,
  'manual_review' => KycStatus.manualReview,
  'rejected' => KycStatus.rejected,
  _ => throw FormatException('kyc status: $s'),
};

/// Where the player's declared identity stands (owner, 2026-10-31): nothing given, given but not checked, an identity
/// submission made, or verified.
enum IdentityStatus { none, declared, submitted, verified }

IdentityStatus identityStatusFrom(String s) => switch (s) {
  'none' => IdentityStatus.none,
  'declared' => IdentityStatus.declared,
  'submitted' => IdentityStatus.submitted,
  'verified' => IdentityStatus.verified,
  _ => throw FormatException('identity status: $s'),
};

class AccountIdentity {
  const AccountIdentity({required this.status, required this.nationalIdHint});

  factory AccountIdentity.fromJson(JsonObject j) =>
      AccountIdentity(status: identityStatusFrom(j.str('status')), nationalIdHint: j.strOrNull('national_id_hint'));

  final IdentityStatus status;

  /// The last three digits of the ID number given, never the number itself; null when none was given.
  final String? nationalIdHint;

  /// "•••••678", or null when no number was given.
  String? get maskedNationalId => nationalIdHint == null ? null : '•••••$nationalIdHint';
}

class Restriction {
  const Restriction({required this.kind, required this.until});

  factory Restriction.fromJson(JsonObject j) => Restriction(kind: j.str('kind'), until: j.instantOrNull('until'));

  /// `timeout` or `self_exclusion`.
  final String kind;

  /// Null for a permanent self-exclusion.
  final DateTime? until;

  bool get selfExclusion => kind == 'self_exclusion';
}

class Account {
  const Account({
    required this.userId,
    required this.msisdn,
    required this.phoneVerified,
    required this.kycStatus,
    required this.status,
    required this.createdAt,
    required this.restriction,
    required this.closureRequestedAt,
    this.mustChangePassword = false,
    this.email,
    this.emailVerified = false,
    this.hasPassword = true,
    this.photoUrl,
    this.firstName,
    this.secondName,
    this.identity,
  });

  factory Account.fromJson(JsonObject j) => Account(
    mustChangePassword: j.raw['must_change_password'] == true,
    email: j.strOrNull('email'),
    emailVerified: j.raw['email_verified'] == true,
    hasPassword: j.raw['has_password'] != false,
    photoUrl: switch (j.strOrNull('photo_url')) {
      // only Google's image host, as the server keeps it (ADR 0035)
      final u? when u.startsWith('https://') && Uri.tryParse(u)?.host.endsWith('.googleusercontent.com') == true => u,
      _ => null,
    },
    firstName: j.strOrNull('first_name'),
    secondName: j.strOrNull('second_name'),
    identity: switch (j.objectOrNull('identity')) {
      final i? => AccountIdentity.fromJson(i),
      null => null,
    },
    userId: j.str('user_id'),
    msisdn: j.str('msisdn'),
    phoneVerified: j.boolean('phone_verified'),
    kycStatus: kycStatusFrom(j.str('kyc_status')),
    status: j.str('status'),
    createdAt: j.instant('created_at'),
    restriction: switch (j.objectOrNull('restriction')) {
      final r? => Restriction.fromJson(r),
      null => null,
    },
    closureRequestedAt: j.instantOrNull('closure_requested_at'),
  );

  final String userId;

  /// 254XXXXXXXXX.
  final String msisdn;
  final bool phoneVerified;
  final KycStatus kycStatus;

  /// active, suspended or closed.
  final String status;
  final DateTime createdAt;
  final Restriction? restriction;
  final DateTime? closureRequestedAt;

  /// Staff gave the player a temporary password they have not replaced yet.
  final bool mustChangePassword;

  /// Where a forgotten password's reset link goes; null for an account from before addresses were asked for.
  final String? email;

  /// Whether the player opened the link sent to that address (ADR 0040). A changed address starts unconfirmed.
  final bool emailVerified;

  /// An address is on the account and its link has not been opened yet.
  bool get emailNeedsConfirming => email != null && !emailVerified;

  /// False for an account that signs in with Google or Apple only.
  final bool hasPassword;

  /// The photo of the Google account the player signs in with (ADR 0035); null otherwise.
  final String? photoUrl;

  /// The first two words of the player's name: from their identity submission, else what they declared at sign-up or
  /// from their account; null for an account that gave neither.
  final String? firstName;
  final String? secondName;

  /// Where the declared identity stands; null when the server has no identity service.
  final AccountIdentity? identity;

  /// The account has no names or ID number yet (one from before sign-up asked for them): the app asks for them.
  bool get needsIdentity => identity?.status == IdentityStatus.none;

  /// "JW" from the names, for the avatar; null without names.
  String? get initials {
    final letters = [?firstName, ?secondName].where((p) => p.isNotEmpty).map((p) => p[0].toUpperCase()).join();
    return letters.isEmpty ? null : letters;
  }

  /// "First Second", or null when the account has no names yet.
  String? get displayName {
    final parts = [?firstName, ?secondName].where((p) => p.isNotEmpty).toList();
    return parts.isEmpty ? null : parts.join(' ');
  }
}

class DepositLimit {
  const DepositLimit({required this.amountKes, required this.effectiveFrom});

  factory DepositLimit.fromJson(JsonObject j) =>
      DepositLimit(amountKes: j.integer('amount_kes'), effectiveFrom: j.instant('effective_from'));

  final int amountKes;
  final DateTime effectiveFrom;
}

class LimitState {
  const LimitState({this.current, this.pending});

  final DepositLimit? current;

  /// An increase waiting out its cooling-off.
  final DepositLimit? pending;
}

/// Deposit limits by period (`daily`, `weekly`, `monthly`).
Map<String, LimitState> limitsFromJson(JsonObject j) {
  final raw = j.object('limits').raw;
  return {
    for (final MapEntry(:key, :value) in raw.entries)
      if (value is Map<String, Object?>)
        key: LimitState(
          current: switch (JsonObject(value).objectOrNull('current')) {
            final c? => DepositLimit.fromJson(c),
            null => null,
          },
          pending: switch (JsonObject(value).objectOrNull('pending')) {
            final p? => DepositLimit.fromJson(p),
            null => null,
          },
        ),
  };
}

/// What a player gave about themself at sign-up (owner, 2026-10-30).
class DeclaredIdentity {
  const DeclaredIdentity({required this.firstName, required this.secondName, required this.nationalId});

  final String firstName;
  final String secondName;
  final String nationalId;

  String get fullName => '$firstName $secondName';
}

/// One of the devices signed in to the account (owner, 2026-10-31).
class DeviceSession {
  const DeviceSession({
    required this.id,
    required this.platform,
    required this.deviceVerdict,
    required this.createdAt,
    required this.lastUsedAt,
    required this.current,
  });

  factory DeviceSession.fromJson(JsonObject j) => DeviceSession(
    id: j.str('id'),
    platform: j.str('platform'),
    deviceVerdict: j.str('device_verdict'),
    createdAt: j.instant('created_at'),
    lastUsedAt: j.instant('last_used_at'),
    current: j.boolean('current'),
  );

  final String id;

  /// android, ios or web.
  final String platform;

  /// The device's attestation verdict: none, passed or failed.
  final String deviceVerdict;
  final DateTime createdAt;
  final DateTime lastUsedAt;

  /// The session this app is using.
  final bool current;
}

/// A Google or Apple identity that signs in to the account (ADR 0020).
class LinkedSignIn {
  const LinkedSignIn({required this.provider, required this.linkedAt});

  factory LinkedSignIn.fromJson(JsonObject j) =>
      LinkedSignIn(provider: j.str('provider'), linkedAt: j.instant('linked_at'));

  final String provider;
  final DateTime linkedAt;
}

/// How the account can be signed in to.
class SignInMethods {
  const SignInMethods({required this.password, required this.providers});

  factory SignInMethods.fromJson(JsonObject j) =>
      SignInMethods(password: j.boolean('password'), providers: j.list('providers', LinkedSignIn.fromJson));

  final bool password;
  final List<LinkedSignIn> providers;

  LinkedSignIn? linked(String provider) => providers.where((p) => p.provider == provider).firstOrNull;
}

/// Device attestation evidence for sign-in (App Attest on iOS, Play Integrity on Android), made for a challenge the
/// server issued. The server alone decides what a token is worth.
///
/// No native attestor is wired yet: it needs the App Attest capability (Apple developer account) and a Play Console
/// app linked to a Cloud project. Until then [NoDeviceAttestation] is used and sign-in sends no token.
abstract interface class DeviceAttestation {
  bool get available;

  /// A token for [challenge], or null when the platform could not produce one.
  Future<String?> attest(String challenge);
}

class NoDeviceAttestation implements DeviceAttestation {
  const NoDeviceAttestation();

  @override
  bool get available => false;

  @override
  Future<String?> attest(String challenge) async => null;
}

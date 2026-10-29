import 'package:flutter/widgets.dart';
import 'package:google_sign_in_web/web_only.dart' as gsi;

/// Google Identity Services renders its own button on the web and will not hand out an ID token any other way.
Widget? googleButton() => gsi.renderButton(
  configuration: gsi.GSIButtonConfiguration(
    theme: gsi.GSIButtonTheme.outline,
    size: gsi.GSIButtonSize.large,
    text: gsi.GSIButtonText.continueWith,
    shape: gsi.GSIButtonShape.rectangular,
    logoAlignment: gsi.GSIButtonLogoAlignment.left,
    minimumWidth: 320,
  ),
);

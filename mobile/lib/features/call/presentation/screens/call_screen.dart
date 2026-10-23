import 'dart:async';
import 'package:flutter/material.dart';
import '../../../../core/constants/app_colors.dart';
import '../../../../shared/widgets/user_avatar.dart';

enum CallType { voice, video }

class CallScreen extends StatefulWidget {
  final String peerName;
  final String? peerAvatarUrl;
  final CallType callType;
  final bool isIncoming;

  const CallScreen({
    super.key,
    required this.peerName,
    this.peerAvatarUrl,
    this.callType = CallType.video,
    this.isIncoming = false,
  });

  @override
  State<CallScreen> createState() => _CallScreenState();
}

class _CallScreenState extends State<CallScreen> {
  bool _isMicMuted = false;
  bool _isCameraOff = false;
  bool _isSpeakerOn = true;
  bool _isCallConnected = false;
  int _callDurationSeconds = 0;
  Timer? _callTimer;

  @override
  void initState() {
    super.initState();
    // Simulate call connecting after 2 seconds
    Future.delayed(const Duration(milliseconds: 2000), () {
      if (mounted) {
        setState(() {
          _isCallConnected = true;
        });
        _startTimer();
      }
    });
  }

  void _startTimer() {
    _callTimer = Timer.periodic(const Duration(seconds: 1), (timer) {
      if (mounted) {
        setState(() {
          _callDurationSeconds++;
        });
      }
    });
  }

  @override
  void dispose() {
    _callTimer?.cancel();
    super.dispose();
  }

  String _formatDuration(int totalSeconds) {
    final minutes = (totalSeconds / 60).floor().toString().padLeft(2, '0');
    final seconds = (totalSeconds % 60).toString().padLeft(2, '0');
    return '$minutes:$seconds';
  }

  void _endCall() {
    _callTimer?.cancel();
    Navigator.pop(context);
  }

  @override
  Widget build(BuildContext context) {
    final isVideo = widget.callType == CallType.video;

    return Scaffold(
      backgroundColor: Colors.black,
      body: Stack(
        children: [
          // 1. Background Video / Audio View
          if (isVideo && !_isCameraOff && _isCallConnected)
            Container(
              width: double.infinity,
              height: double.infinity,
              decoration: BoxDecoration(
                gradient: LinearGradient(
                  colors: [
                    Colors.black,
                    AppColors.primaryDark.withOpacity(0.3),
                    Colors.black,
                  ],
                  begin: Alignment.topCenter,
                  end: Alignment.bottomCenter,
                ),
              ),
              child: const Center(
                child: Icon(
                  Icons.videocam_rounded,
                  size: 72,
                  color: Colors.white24,
                ),
              ),
            )
          else
            // Voice Call / Audio Visualizer
            Center(
              child: Column(
                mainAxisAlignment: MainAxisAlignment.center,
                children: [
                  Container(
                    padding: const EdgeInsets.all(8),
                    decoration: BoxDecoration(
                      shape: BoxShape.circle,
                      boxShadow: [
                        BoxShadow(
                          color: AppColors.primary.withOpacity(0.25),
                          blurRadius: 40,
                          spreadRadius: 10,
                        ),
                      ],
                    ),
                    child: UserAvatar(
                      imageUrl: widget.peerAvatarUrl,
                      name: widget.peerName,
                      radius: 64,
                      hasStory: true,
                    ),
                  ),
                  const SizedBox(height: 24),
                  Text(
                    widget.peerName,
                    style: const TextStyle(
                      color: Colors.white,
                      fontSize: 26,
                      fontWeight: FontWeight.bold,
                      letterSpacing: -0.3,
                    ),
                  ),
                  const SizedBox(height: 8),
                  Text(
                    _isCallConnected
                        ? _formatDuration(_callDurationSeconds)
                        : (widget.isIncoming ? 'Incoming call...' : 'Calling...'),
                    style: TextStyle(
                      color: _isCallConnected ? AppColors.onlineGreen : AppColors.textSecondaryDark,
                      fontSize: 16,
                      fontWeight: FontWeight.w600,
                    ),
                  ),
                ],
              ),
            ),

          // 2. Picture-in-Picture Local Camera (if Video Call)
          if (isVideo && _isCallConnected)
            Positioned(
              top: 50,
              right: 18,
              child: Container(
                width: 110,
                height: 160,
                decoration: BoxDecoration(
                  color: AppColors.darkCard,
                  borderRadius: BorderRadius.circular(20),
                  border: Border.all(
                    color: AppColors.primary.withOpacity(0.4),
                    width: 1.5,
                  ),
                  boxShadow: [
                    BoxShadow(
                      color: Colors.black.withOpacity(0.5),
                      blurRadius: 16,
                    ),
                  ],
                ),
                child: ClipRRect(
                  borderRadius: BorderRadius.circular(19),
                  child: Stack(
                    alignment: Alignment.center,
                    children: [
                      Container(
                        color: const Color(0xFF161824),
                        child: const Center(
                          child: Icon(Icons.person, color: Colors.white38, size: 36),
                        ),
                      ),
                      Positioned(
                        bottom: 6,
                        child: Container(
                          padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
                          decoration: BoxDecoration(
                            color: Colors.black54,
                            borderRadius: BorderRadius.circular(6),
                          ),
                          child: const Text(
                            'You',
                            style: TextStyle(color: Colors.white, fontSize: 10),
                          ),
                        ),
                      ),
                    ],
                  ),
                ),
              ),
            ),

          // 3. Floating Bottom Controls Bar
          Positioned(
            bottom: 40,
            left: 20,
            right: 20,
            child: Container(
              padding: const EdgeInsets.symmetric(vertical: 14, horizontal: 16),
              decoration: BoxDecoration(
                color: const Color(0xFF161824).withOpacity(0.92),
                borderRadius: BorderRadius.circular(36),
                border: Border.all(color: AppColors.darkBorder, width: 0.8),
                boxShadow: [
                  BoxShadow(
                    color: Colors.black.withOpacity(0.6),
                    blurRadius: 30,
                    offset: const Offset(0, 10),
                  ),
                ],
              ),
              child: Row(
                mainAxisAlignment: MainAxisAlignment.spaceEvenly,
                children: [
                  // Mic Mute Toggle
                  _buildCallActionBtn(
                    icon: _isMicMuted ? Icons.mic_off_rounded : Icons.mic_rounded,
                    isActive: _isMicMuted,
                    activeColor: AppColors.error,
                    onTap: () {
                      setState(() => _isMicMuted = !_isMicMuted);
                    },
                  ),

                  // Speaker Toggle
                  _buildCallActionBtn(
                    icon: _isSpeakerOn ? Icons.volume_up_rounded : Icons.volume_off_rounded,
                    isActive: _isSpeakerOn,
                    activeColor: AppColors.primary,
                    onTap: () {
                      setState(() => _isSpeakerOn = !_isSpeakerOn);
                    },
                  ),

                  // Camera Toggle (if Video)
                  if (isVideo)
                    _buildCallActionBtn(
                      icon: _isCameraOff ? Icons.videocam_off_rounded : Icons.videocam_rounded,
                      isActive: _isCameraOff,
                      activeColor: AppColors.accent,
                      onTap: () {
                        setState(() => _isCameraOff = !_isCameraOff);
                      },
                    ),

                  // End Call Button (Big Red Button)
                  GestureDetector(
                    onTap: _endCall,
                    child: Container(
                      width: 52,
                      height: 52,
                      decoration: BoxDecoration(
                        color: AppColors.error,
                        shape: BoxShape.circle,
                        boxShadow: [
                          BoxShadow(
                            color: AppColors.error.withOpacity(0.4),
                            blurRadius: 18,
                            offset: const Offset(0, 4),
                          ),
                        ],
                      ),
                      child: const Icon(
                        Icons.call_end_rounded,
                        color: Colors.white,
                        size: 26,
                      ),
                    ),
                  ),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildCallActionBtn({
    required IconData icon,
    required bool isActive,
    required Color activeColor,
    required VoidCallback onTap,
  }) {
    return GestureDetector(
      onTap: onTap,
      child: Container(
        width: 48,
        height: 48,
        decoration: BoxDecoration(
          color: isActive ? activeColor.withOpacity(0.2) : Colors.white.withOpacity(0.08),
          shape: BoxShape.circle,
          border: Border.all(
            color: isActive ? activeColor : Colors.white12,
            width: 1.2,
          ),
        ),
        child: Icon(
          icon,
          color: isActive ? activeColor : Colors.white,
          size: 22,
        ),
      ),
    );
  }
}

class WebSocketClient {
    constructor(onMessage, onCallSignal) {
        this.ws = null;
        this.onMessage = onMessage;
        this.onCallSignal = onCallSignal;
        this.reconnectAttempts = 0;
        this.maxReconnectAttempts = 10;
        this.reconnectDelay = 1000;
        this.isIntentionallyClosed = false;
    }

    connect() {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/ws/chat`;
        
        console.log('WebSocket: Connecting to', wsUrl);
        
        try {
            this.ws = new WebSocket(wsUrl);
            this.setupEventHandlers();
        } catch (error) {
            console.error('WebSocket: Connection error', error);
            this.scheduleReconnect();
        }
    }

    setupEventHandlers() {
        this.ws.onopen = () => {
            console.log('WebSocket: Connected');
            this.reconnectAttempts = 0;
            this.updateStatus('Connected', 'text-green-500');
            this.sendPing();
        };

        this.ws.onmessage = (event) => {
            try {
                const message = JSON.parse(event.data);
                this.handleMessage(message);
            } catch (error) {
                console.error('WebSocket: Failed to parse message', error);
            }
        };

        this.ws.onerror = (error) => {
            console.error('WebSocket: Error', error);
            this.updateStatus('Error', 'text-red-500');
        };

        this.ws.onclose = (event) => {
            console.log('WebSocket: Closed', event.code, event.reason);
            
            if (!this.isIntentionallyClosed) {
                this.updateStatus('Reconnecting...', 'text-yellow-500');
                this.scheduleReconnect();
            }
        };
    }

    handleMessage(message) {
        console.log('WebSocket: Message received', message.type);
        
        switch (message.type) {
            case 'chat':
            case 'group_chat':
                if (this.onMessage) {
                    this.onMessage(message);
                }
                break;
            case 'notification':
                if (this.onMessage) {
                    this.onMessage(message);
                }
                break;
            case 'call_offer':
            case 'call_answer':
            case 'call_ice':
            case 'call_end':
            case 'call_ringing':
                if (this.onCallSignal) {
                    this.onCallSignal(message);
                }
                break;
                
            case 'ping':
                this.sendPong();
                break;
                
            case 'pong':
                // Keep-alive acknowledged
                break;
                
            default:
                console.warn('WebSocket: Unknown message type', message.type);
        }
    }

    sendMessage(type, payload) {
        if (this.ws && this.ws.readyState === WebSocket.OPEN) {
            const message = {
                type: type,
                data: payload,
                timestamp: Math.floor(Date.now() / 1000)
            };
            
            if (payload.to) {
                message.to = payload.to;
            }
            if (payload.group_id) {
                message.group_id = payload.group_id;
            }
            
            this.ws.send(JSON.stringify(message));
            return true;
        }
        
        console.error('WebSocket: Cannot send, not connected');
        return false;
    }

    sendPing() {
        this.sendMessage('ping', {});
    }

    sendPong() {
        this.sendMessage('pong', {});
    }

    updateStatus(status, colorClass) {
        const statusEl = document.getElementById('connection-status');
        if (statusEl) {
            statusEl.textContent = status;
            statusEl.className = 'text-xs ' + colorClass;
        }
    }

    scheduleReconnect() {
        if (this.reconnectAttempts >= this.maxReconnectAttempts) {
            console.error('WebSocket: Max reconnection attempts reached');
            this.updateStatus('Disconnected', 'text-red-500');
            return;
        }

        this.reconnectAttempts++;
        const delay = Math.min(
            this.reconnectDelay * Math.pow(2, this.reconnectAttempts),
            30000
        );
        
        console.log(`WebSocket: Reconnecting in ${delay}ms (attempt ${this.reconnectAttempts})`);
        
        setTimeout(() => {
            if (!this.isIntentionallyClosed) {
                this.connect();
            }
        }, delay);
    }

    close() {
        this.isIntentionallyClosed = true;
        if (this.ws) {
            this.ws.close();
        }
    }
}

// WebRTC Voice Call Manager with Firefox Support
class VoiceCallManager {
    constructor(wsClient, username) {
        this.wsClient = wsClient;
        this.username = username;
        this.pc = null;
        this.localStream = null;
        this.remoteStream = null;
        this.currentCallId = null;
        this.currentCallPeer = null;
        
        // Inject custom CSS for animations
        this.injectStyles();

        this.isWebRTCSupported = this.detectWebRTCSupport();
        
        if (!this.isWebRTCSupported) {
            console.warn('WebRTC is not fully supported in this browser context');
        }
        
        this.iceServers = {
            iceServers: [
                { urls: 'stun:stun.l.google.com:19302' },
                { urls: 'stun:stun1.l.google.com:19302' }
            ]
        };
    }

    injectStyles() {
        const styleId = 'voice-call-styles';
        if (document.getElementById(styleId)) return;

        const style = document.createElement('style');
        style.id = styleId;
        style.textContent = `
            @keyframes ring-ripple {
                0% { box-shadow: 0 0 0 0 rgba(59, 130, 246, 0.4); }
                70% { box-shadow: 0 0 0 20px rgba(59, 130, 246, 0); }
                100% { box-shadow: 0 0 0 0 rgba(59, 130, 246, 0); }
            }
            @keyframes ring-ripple-red {
                0% { box-shadow: 0 0 0 0 rgba(239, 68, 68, 0.4); }
                70% { box-shadow: 0 0 0 20px rgba(239, 68, 68, 0); }
                100% { box-shadow: 0 0 0 0 rgba(239, 68, 68, 0); }
            }
            @keyframes slide-up-fade {
                0% { opacity: 0; transform: translateY(20px) scale(0.95); }
                100% { opacity: 1; transform: translateY(0) scale(1); }
            }
            .animate-ring-ripple { animation: ring-ripple 1.5s infinite; }
            .animate-ring-ripple-red { animation: ring-ripple-red 1.5s infinite; }
            .animate-slide-in { animation: slide-up-fade 0.3s cubic-bezier(0.16, 1, 0.3, 1); }
            
            .glass-panel {
                background: rgba(30, 30, 30, 0.85);
                backdrop-filter: blur(12px);
                border: 1px solid rgba(255, 255, 255, 0.1);
                box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.5);
            }
        `;
        document.head.appendChild(style);
    }
    
    detectWebRTCSupport() {
        // Check RTCPeerConnection (Modern & Legacy)
        const hasRTCPeerConnection = !!(
            window.RTCPeerConnection ||
            window.webkitRTCPeerConnection ||
            window.mozRTCPeerConnection
        );
        
        // Check getUserMedia (Modern & Legacy)
        // Firefox hides 'mediaDevices' on insecure HTTP, causing the original check to fail
        const hasGetUserMedia = !!(
            (navigator.mediaDevices && navigator.mediaDevices.getUserMedia) ||
            navigator.webkitGetUserMedia ||
            navigator.mozGetUserMedia ||
            navigator.msGetUserMedia
        );
        
        // Check secure context explicitly
        const isSecureContext = window.isSecureContext || location.protocol === 'https:' || location.hostname === 'localhost';
        
        const isSupported = hasRTCPeerConnection && hasGetUserMedia;

        // Enhanced Debugging for Firefox
        if (!isSupported) {
            console.group('WebRTC Support Debug');
            console.log('RTCPeerConnection:', hasRTCPeerConnection);
            console.log('getUserMedia:', hasGetUserMedia);
            console.log('isSecureContext:', isSecureContext);
            console.log('Protocol:', location.protocol);
            
            if (location.protocol === 'http:' && location.hostname !== 'localhost') {
                console.error('FIREFOX ERROR: Firefox disables WebRTC APIs on HTTP connection to IP addresses. Use HTTPS or localhost.');
            }
            console.groupEnd();
        }
        
        return isSupported;
    }
    
    // 2. Cross-Browser getUserMedia Helper
    async getUserMediaCompat(constraints) {
        // Modern API
        if (navigator.mediaDevices && navigator.mediaDevices.getUserMedia) {
            return navigator.mediaDevices.getUserMedia(constraints);
        }
        
        // Legacy API (Polyfill for older Firefox/Chrome)
        const legacyGetUserMedia = navigator.webkitGetUserMedia || navigator.mozGetUserMedia || navigator.msGetUserMedia;
        
        if (legacyGetUserMedia) {
            return new Promise((resolve, reject) => {
                legacyGetUserMedia.call(navigator, constraints, resolve, reject);
            });
        }
        
        throw new Error('getUserMedia is not supported in this browser');
    }

    // Helper to detect browser
    detectBrowser() {
        const ua = navigator.userAgent;
        if (ua.includes('Firefox')) return 'Firefox';
        if (ua.includes('Chrome') && !ua.includes('Edg')) return 'Chrome';
        if (ua.includes('Safari') && !ua.includes('Chrome')) return 'Safari';
        if (ua.includes('Edg')) return 'Edge';
        return 'Unknown';
    }

    getCSRFToken() {
        const meta = document.querySelector('meta[name="csrf-token"]');
        return meta ? meta.getAttribute('content') : '';
    }

    async initiateCall(targetUsername) {
        try {
            // Check WebRTC support
            if (!this.isWebRTCSupported) {
                // Provide helpful error based on browser and protocol
                const browser = this.detectBrowser();
                const isFirefox = browser === 'Firefox';
                const isHTTP = location.protocol === 'http:';
                
                let errorMessage = 'WebRTC is not supported in your browser.';
                
                if (isFirefox && isHTTP) {
                    errorMessage = `Firefox requires HTTPS for voice calls.
To enable voice calls:
1. Use HTTPS (recommended for production)
2. Or use Chrome for local testing (supports HTTP on localhost)
Technical details: Firefox disables WebRTC on insecure (HTTP) connections for security reasons.`;
                } else if (isHTTP) {
                    errorMessage = `Voice calls require a secure connection (HTTPS).
Your browser blocks WebRTC on HTTP for security.
Please use HTTPS or test with Chrome which allows localhost over HTTP.`;
                }
                
                throw new Error(errorMessage);
            }
            
            console.log('Requesting microphone access...');
            
            // Get user media (microphone)
            this.localStream = await this.getUserMediaCompat({
                audio: true,
                video: false
            });
            
            console.log('Got local stream:', this.localStream);
            
            // Handle different prefixes for PeerConnection
            const RTCPeerConnection = window.RTCPeerConnection ||
                                    window.webkitRTCPeerConnection ||
                                    window.mozRTCPeerConnection;
            
            this.pc = new RTCPeerConnection(this.iceServers);
            this.setupPeerConnection();
            
            this.localStream.getTracks().forEach(track => {
                this.pc.addTrack(track, this.localStream);
            });
            
            // Call backend to initiate call
            const response = await fetch(`/call/initiate/${targetUsername}`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'X-CSRF-Token': this.getCSRFToken()
                }
            });
            
            if (!response.ok) throw new Error('Failed to initiate call');
            const data = await response.json();
            
            this.currentCallId = data.call_id;
            this.currentCallPeer = targetUsername;
            this.isInitiator = true;
            
            const offer = await this.pc.createOffer();
            await this.pc.setLocalDescription(offer);
            
            this.wsClient.sendMessage('call_offer', {
                call_id: this.currentCallId,
                to: targetUsername,
                sdp: offer.sdp
            });
            
            this.showCallingUI(targetUsername);
            
        } catch (error) {
            console.error('Failed to initiate call:', error);
            this.endCall();
            alert('Failed to start call: ' + error.message);
        }
    }

    async answerCall() {
        try {
            if (!this.isWebRTCSupported) {
                throw new Error('WebRTC is not supported in your browser.');
            }

            // Verify we actually received an offer
            if (!this.pendingOffer) {
                console.warn("No pending offer found. Waiting for SDP...");
                // In a real app, you might want to show a 'Connecting...' state here
                throw new Error("Cannot answer: connection offer not received yet."); 
            }
            
            console.log('Requesting microphone access...');
            
            // Get user media (microphone)
            this.localStream = await navigator.mediaDevices.getUserMedia({
                audio: true,
                video: false
            });
            
            // Create peer connection
            const RTCPeerConnection = window.RTCPeerConnection ||
                                     window.webkitRTCPeerConnection ||
                                     window.mozRTCPeerConnection;
            
            this.pc = new RTCPeerConnection(this.iceServers);
            this.setupPeerConnection();
            
            // Add local stream
            this.localStream.getTracks().forEach(track => {
                this.pc.addTrack(track, this.localStream);
            });

            // FIX: Set Remote Description (The Offer) BEFORE creating Answer
            console.log("Setting remote description (Offer)...");
            await this.pc.setRemoteDescription(new RTCSessionDescription({
                type: 'offer',
                sdp: this.pendingOffer
            }));
            
            // Answer the call (API)
            // Ensure you have the getCSRFToken helper method in your class!
            const response = await fetch(`/call/answer/${this.currentCallId}`, {
                method: 'POST',
                headers: {
                    'X-CSRF-Token': this.getCSRFToken() 
                }
            });
            
            if (!response.ok) {
                throw new Error('Failed to answer call');
            }
            
            // Create answer
            const answer = await this.pc.createAnswer();
            await this.pc.setLocalDescription(answer);
            
            // Send answer via WebSocket
            this.wsClient.sendMessage('call_answer', {
                call_id: this.currentCallId,
                to: this.currentCallPeer,
                sdp: answer.sdp
            });
            
            this.showActiveCallUI();
            
        } catch (error) {
            console.error('Failed to answer call:', error);
            this.endCall();
            alert('Failed to answer call: ' + error.message);
        }
    }

    async rejectCall() {
        try {
            await fetch(`/call/reject/${this.currentCallId}`, {
                method: 'POST',
                headers: {
                    'X-CSRF-Token': this.getCSRFToken()
                }
            });
            
            this.cleanup();
            this.hideCallUI();
            
        } catch (error) {
            console.error('Failed to reject call:', error);
        }
    }

    async endCall() {
        try {
            if (this.currentCallId) {
                await fetch(`/call/end/${this.currentCallId}`, {
                    method: 'POST',
                    headers: {
                        'X-CSRF-Token': this.getCSRFToken()
                    }
                });
            }
            
            this.cleanup();
            this.hideCallUI();
            
        } catch (error) {
            console.error('Failed to end call:', error);
            this.cleanup();
            this.hideCallUI();
        }
    }

    setupPeerConnection() {
        // Handle ICE candidates
        this.pc.onicecandidate = (event) => {
            if (event.candidate) {
                console.log('New ICE candidate:', event.candidate);
                
                // Send ICE candidate via WebSocket
                this.wsClient.sendMessage('call_ice', {
                    call_id: this.currentCallId,
                    to: this.currentCallPeer,
                    candidate: {
                        candidate: event.candidate.candidate,
                        sdpMLineIndex: event.candidate.sdpMLineIndex,
                        sdpMid: event.candidate.sdpMid
                    }
                });
            }
        };

        // Handle remote stream
        this.pc.ontrack = (event) => {
            console.log('Received remote track:', event.streams[0]);
            this.remoteStream = event.streams[0];
            
            // Play remote audio
            const remoteAudio = document.getElementById('remote-audio');
            if (remoteAudio) {
                remoteAudio.srcObject = this.remoteStream;
                remoteAudio.play();
            }
        };

        // Handle connection state changes
        this.pc.onconnectionstatechange = () => {
            console.log('Connection state:', this.pc.connectionState);
            
            if (this.pc.connectionState === 'connected') {
                this.showActiveCallUI();
            } else if (this.pc.connectionState === 'failed' || 
                       this.pc.connectionState === 'disconnected') {
                this.endCall();
            }
        };
    }

    async handleCallSignal(message) {
        switch (message.type) {
            case 'call_offer':
                await this.handleIncomingCall(message);
                break;
                
            case 'call_answer':
                await this.handleCallAnswer(message);
                break;
                
            case 'call_ice':
                await this.handleICECandidate(message);
                break;
                
            case 'call_end':
                this.handleCallEnd(message);
                break;
                
            case 'call_ringing':
                console.log('Call is ringing');
                break;
        }
    }

    async handleIncomingCall(message) {
        // Safety check to prevent crash
        if (!message.data) {
            console.warn("Received call signal without data:", message);
            return;
        }

        this.currentCallId = message.data.call_id;
        this.currentCallPeer = message.from;
        
        // Show incoming call UI
        this.showIncomingCallUI(message.from);
        
        // This handles the race condition between the Server's "Ringing" message (No SDP)
        // and the Client's "Offer" message (Has SDP).
        if (message.data.sdp) {
            console.log("Received SDP Offer");
            this.pendingOffer = message.data.sdp;
        }
    }

    async handleCallAnswer(message) {
        if (!this.pc) return;
        
        try {
            await this.pc.setRemoteDescription(
                new RTCSessionDescription({
                    type: 'answer',
                    sdp: message.data.sdp
                })
            );
            
            console.log('Remote description set');
        } catch (error) {
            console.error('Failed to set remote description:', error);
        }
    }

    async handleICECandidate(message) {
        if (!this.pc) return;
        
        try {
            const candidate = new RTCIceCandidate(message.data.candidate);
            await this.pc.addIceCandidate(candidate);
            console.log('ICE candidate added');
        } catch (error) {
            console.error('Failed to add ICE candidate:', error);
        }
    }

    handleCallEnd(message) {
        console.log('Call ended by', message.from);
        this.cleanup();
        this.hideCallUI();
        
        // Show stylized toast instead of alert
        const reason = message.data && message.data.rejected ? 'Call Rejected' : 'Call Ended';
        this.showToast(reason, message.from, 'neutral');
    }

    showToast(title, subtitle, type = 'neutral') {
        const toast = document.createElement('div');
        toast.className = `fixed top-6 left-1/2 transform -translate-x-1/2 z-[60] flex items-center gap-3 px-6 py-4 rounded-full shadow-2xl animate-slide-in border border-white/10 ${
            type === 'error' ? 'bg-red-900/90 text-red-100' : 'bg-gray-900/90 text-white'
        } backdrop-blur-md min-w-[300px] justify-center`;
        
        const container = document.createElement('div');
        container.className = 'flex flex-col items-center text-center';

        const titleSpan = document.createElement('span');
        titleSpan.className = 'font-bold text-sm tracking-wide';
        titleSpan.textContent = title;
        container.appendChild(titleSpan);

        if (subtitle) {
            const subtitleSpan = document.createElement('span');
            subtitleSpan.className = 'text-xs opacity-70 mt-0.5';
            subtitleSpan.textContent = subtitle;
            container.appendChild(subtitleSpan);
        }

        toast.appendChild(container);

        document.body.appendChild(toast);

        // Remove after 3 seconds
        setTimeout(() => {
            toast.style.transition = 'all 0.5s ease';
            toast.style.opacity = '0';
            toast.style.transform = 'translate(-50%, -20px)';
            setTimeout(() => toast.remove(), 500);
        }, 3000);
    }

    showIncomingCallUI(caller) {
        let modal = document.getElementById('incoming-call-modal');
        if (!modal) {
            modal = this.createIncomingCallModal(caller);
            document.body.appendChild(modal);
        } else {
            document.getElementById('caller-name').textContent = caller;
            modal.classList.remove('hidden');
        }
    }

    createIncomingCallModal(caller) {
        const modal = document.createElement('div');
        modal.id = 'incoming-call-modal';
        modal.className = 'fixed inset-0 bg-black/60 backdrop-blur-sm flex items-center justify-center z-50 transition-all duration-300';
        modal.innerHTML = `
            <div class="glass-panel rounded-3xl p-8 max-w-sm w-full mx-4 animate-slide-in relative overflow-hidden">
                <div class="absolute top-0 left-1/2 -translate-x-1/2 w-32 h-32 bg-blue-500/20 rounded-full blur-3xl -z-10"></div>
                
                <div class="text-center">
                    <div class="relative inline-block">
                        <div class="w-24 h-24 bg-gradient-to-br from-blue-500 to-blue-600 rounded-full flex items-center justify-center mx-auto mb-6 shadow-lg shadow-blue-500/30 animate-ring-ripple">
                            <svg class="w-10 h-10 text-white drop-shadow-md" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 5a2 2 0 012-2h3.28a1 1 0 01.948.684l1.498 4.493a1 1 0 01-.502 1.21l-2.257 1.13a11.042 11.042 0 005.516 5.516l1.13-2.257a1 1 0 011.21-.502l4.493 1.498a1 1 0 01.684.949V19a2 2 0 01-2 2h-1C9.716 21 3 14.284 3 6V5z"></path>
                            </svg>
                        </div>
                        <span class="absolute top-0 right-0 w-6 h-6 bg-green-500 border-4 border-[#1e1e1e] rounded-full"></span>
                    </div>

                    <h3 id="caller-name" class="text-3xl font-bold text-white mb-1 tracking-tight"></h3>
                    <p class="text-blue-200/70 text-sm font-medium uppercase tracking-widest mb-10">Incoming Call...</p>
                    
                    <div class="flex gap-6 justify-center items-center">
                        <button onclick="window.voiceCall.rejectCall()" class="group flex flex-col items-center gap-2 transition-transform hover:scale-105">
                            <div class="w-16 h-16 rounded-full bg-red-500/10 border border-red-500/50 hover:bg-red-500 hover:border-red-500 flex items-center justify-center transition-all duration-300">
                                <svg class="w-6 h-6 text-red-500 group-hover:text-white transition-colors" fill="currentColor" viewBox="0 0 24 24">
                                    <path d="M12 9c-1.6 0-3.15.25-4.6.72v3.1c0 .39-.23.74-.56.9-.98.49-1.87 1.12-2.66 1.85-.18.18-.43.28-.7.28-.28 0-.53-.11-.71-.29L.29 13.08c-.18-.17-.29-.42-.29-.7 0-.28.11-.53.29-.71C3.34 8.78 7.46 7 12 7s8.66 1.78 11.71 4.67c.18.18.29.43.29.71 0 .28-.11.53-.29.71l-2.48 2.48c-.18.18-.43.29-.71.29-.27 0-.52-.11-.7-.28-.79-.74-1.69-1.36-2.67-1.85-.33-.16-.56-.5-.56-.9v-3.1C15.15 9.25 13.6 9 12 9z"></path>
                                </svg>
                            </div>
                            <span class="text-xs text-gray-400 font-medium">Decline</span>
                        </button>

                        <button onclick="window.voiceCall.answerCall()" class="group flex flex-col items-center gap-2 transition-transform hover:scale-105">
                            <div class="w-20 h-20 rounded-full bg-green-500 hover:bg-green-400 shadow-lg shadow-green-500/30 flex items-center justify-center transition-all duration-300 animate-pulse">
                                <svg class="w-8 h-8 text-white" fill="currentColor" viewBox="0 0 24 24">
                                    <path d="M20.01 15.38c-1.23 0-2.42-.2-3.53-.56a.977.977 0 00-1.01.24l-1.57 1.97c-2.83-1.35-5.48-3.9-6.89-6.83l1.95-1.66c.27-.28.35-.67.24-1.02-.37-1.11-.56-2.3-.56-3.53 0-.54-.45-.99-.99-.99H4.19C3.65 3 3 3.24 3 3.99 3 13.28 10.73 21 20.01 21c.71 0 .99-.63.99-1.18v-3.45c0-.54-.45-.99-.99-.99z"></path>
                                </svg>
                            </div>
                            <span class="text-xs text-green-400 font-medium">Accept</span>
                        </button>
                    </div>
                </div>
            </div>
            <audio id="remote-audio" autoplay></audio>
        `;
        modal.querySelector('#caller-name').textContent = caller;
        return modal;
    }

    showCallingUI(callee) {
        let modal = document.getElementById('calling-modal');
        if (!modal) {
            modal = this.createCallingModal(callee);
            document.body.appendChild(modal);
        } else {
            document.getElementById('callee-name').textContent = callee;
            modal.classList.remove('hidden');
        }
    }

    createCallingModal(callee) {
        const modal = document.createElement('div');
        modal.id = 'calling-modal';
        modal.className = 'fixed inset-0 bg-black/60 backdrop-blur-sm flex items-center justify-center z-50 transition-all';
        modal.innerHTML = `
            <div class="glass-panel rounded-3xl p-10 max-w-sm w-full mx-4 animate-slide-in">
                <div class="text-center">
                     <div class="w-24 h-24 bg-gradient-to-br from-gray-700 to-gray-600 rounded-full flex items-center justify-center mx-auto mb-6 shadow-inner relative">
                        <span class="absolute inset-0 rounded-full border border-white/10 animate-ping opacity-20"></span>
                        <svg class="w-10 h-10 text-white/50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 5a2 2 0 012-2h3.28a1 1 0 01.948.684l1.498 4.493a1 1 0 01-.502 1.21l-2.257 1.13a11.042 11.042 0 005.516 5.516l1.13-2.257a1 1 0 011.21-.502l4.493 1.498a1 1 0 01.684.949V19a2 2 0 01-2 2h-1C9.716 21 3 14.284 3 6V5z"></path>
                        </svg>
                    </div>
                    <h3 id="callee-name" class="text-2xl font-bold text-white mb-2"></h3>
                    <div class="flex items-center justify-center gap-2 mb-10">
                        <span class="w-2 h-2 bg-blue-400 rounded-full animate-bounce"></span>
                        <span class="w-2 h-2 bg-blue-400 rounded-full animate-bounce delay-100"></span>
                        <span class="w-2 h-2 bg-blue-400 rounded-full animate-bounce delay-200"></span>
                    </div>
                    
                    <button onclick="window.voiceCall.endCall()" class="w-full bg-red-500/10 hover:bg-red-500 text-red-500 hover:text-white border border-red-500/30 px-6 py-4 rounded-xl transition-all duration-300 font-medium flex items-center justify-center gap-2 group">
                        <svg class="w-5 h-5 group-hover:rotate-90 transition-transform" fill="currentColor" viewBox="0 0 24 24">
                            <path d="M12 9c-1.6 0-3.15.25-4.6.72v3.1c0 .39-.23.74-.56.9-.98.49-1.87 1.12-2.66 1.85-.18.18-.43.28-.7.28-.28 0-.53-.11-.71-.29L.29 13.08c-.18-.17-.29-.42-.29-.7 0-.28.11-.53.29-.71C3.34 8.78 7.46 7 12 7s8.66 1.78 11.71 4.67c.18.18.29.43.29.71 0 .28-.11.53-.29.71l-2.48 2.48c-.18.18-.43.29-.71.29-.27 0-.52-.11-.7-.28-.79-.74-1.69-1.36-2.67-1.85-.33-.16-.56-.5-.56-.9v-3.1C15.15 9.25 13.6 9 12 9z"></path>
                        </svg>
                        Cancel Call
                    </button>
                </div>
            </div>
            <audio id="remote-audio" autoplay></audio>
        `;
        modal.querySelector('#callee-name').textContent = callee;
        return modal;
    }

    showActiveCallUI() {
        let modal = document.getElementById('active-call-modal');
        if (!modal) {
            modal = this.createActiveCallModal();
            document.body.appendChild(modal);
        }
        modal.classList.remove('hidden');
        
        // Hide other modals
        const incomingModal = document.getElementById('incoming-call-modal');
        if (incomingModal) incomingModal.classList.add('hidden');
        
        const callingModal = document.getElementById('calling-modal');
        if (callingModal) callingModal.classList.add('hidden');
        
        // Start call timer
        this.startCallTimer();
    }

    createActiveCallModal() {
        const modal = document.createElement('div');
        modal.id = 'active-call-modal';
        modal.className = 'fixed inset-0 bg-black/80 backdrop-blur-md flex items-center justify-center z-50';
        modal.innerHTML = `
            <div class="glass-panel rounded-3xl p-8 max-w-sm w-full mx-4 relative overflow-hidden">
                <div class="absolute -top-10 -right-10 w-40 h-40 bg-green-500/10 rounded-full blur-3xl"></div>
                <div class="absolute -bottom-10 -left-10 w-40 h-40 bg-blue-500/10 rounded-full blur-3xl"></div>

                <div class="text-center relative z-10">
                    <div class="mb-6 relative inline-block">
                        <div class="w-24 h-24 rounded-full bg-gradient-to-br from-gray-800 to-gray-700 p-1">
                            <div class="w-full h-full rounded-full bg-gray-800 flex items-center justify-center overflow-hidden">
                                <span class="text-3xl font-bold text-gray-500" id="peer-initial"></span>
                            </div>
                        </div>
                        <span class="absolute bottom-1 right-1 w-5 h-5 bg-green-500 border-4 border-gray-800 rounded-full"></span>
                    </div>

                    <h3 class="text-2xl font-bold text-white mb-1" id="peer-name"></h3>
                    <div class="flex items-center justify-center gap-2 mb-8 opacity-60">
                        <span class="w-1.5 h-1.5 bg-red-500 rounded-full animate-pulse"></span>
                        <p id="call-timer" class="text-sm font-mono tracking-wider">00:00</p>
                    </div>

                    <div class="flex justify-center gap-1 h-8 mb-8 items-center">
                        <div class="w-1 bg-white/40 rounded-full animate-[pulse_1s_ease-in-out_infinite] h-3"></div>
                        <div class="w-1 bg-white/60 rounded-full animate-[pulse_1.2s_ease-in-out_infinite] h-5"></div>
                        <div class="w-1 bg-white/40 rounded-full animate-[pulse_0.8s_ease-in-out_infinite] h-4"></div>
                        <div class="w-1 bg-white/70 rounded-full animate-[pulse_1.5s_ease-in-out_infinite] h-6"></div>
                        <div class="w-1 bg-white/40 rounded-full animate-[pulse_1s_ease-in-out_infinite] h-3"></div>
                    </div>

                    <button onclick="window.voiceCall.endCall()" class="w-16 h-16 bg-red-500 hover:bg-red-600 rounded-full text-white shadow-xl shadow-red-500/20 transition-all hover:scale-110 active:scale-95 flex items-center justify-center mx-auto">
                        <svg class="w-8 h-8" fill="currentColor" viewBox="0 0 24 24">
                            <path d="M12 9c-1.6 0-3.15.25-4.6.72v3.1c0 .39-.23.74-.56.9-.98.49-1.87 1.12-2.66 1.85-.18.18-.43.28-.7.28-.28 0-.53-.11-.71-.29L.29 13.08c-.18-.17-.29-.42-.29-.7 0-.28.11-.53.29-.71C3.34 8.78 7.46 7 12 7s8.66 1.78 11.71 4.67c.18.18.29.43.29.71 0 .28-.11.53-.29.71l-2.48 2.48c-.18.18-.43.29-.71.29-.27 0-.52-.11-.7-.28-.79-.74-1.69-1.36-2.67-1.85-.33-.16-.56-.5-.56-.9v-3.1C15.15 9.25 13.6 9 12 9z"></path>
                        </svg>
                    </button>
                </div>
            </div>
            <audio id="remote-audio" autoplay></audio>
        `;
        modal.querySelector('#peer-initial').textContent = this.currentCallPeer ? this.currentCallPeer.charAt(0).toUpperCase() : '?';
        modal.querySelector('#peer-name').textContent = this.currentCallPeer;
        return modal;
    }

    startCallTimer() {
        if (this.callTimerInterval) {
            clearInterval(this.callTimerInterval);
        }
        
        const startTime = Date.now();
        this.callTimerInterval = setInterval(() => {
            const elapsed = Math.floor((Date.now() - startTime) / 1000);
            const minutes = Math.floor(elapsed / 60).toString().padStart(2, '0');
            const seconds = (elapsed % 60).toString().padStart(2, '0');
            
            const timerEl = document.getElementById('call-timer');
            if (timerEl) {
                timerEl.textContent = `${minutes}:${seconds}`;
            }
        }, 1000);
    }

    hideCallUI() {
        const modals = [
            'incoming-call-modal',
            'calling-modal',
            'active-call-modal'
        ];
        
        modals.forEach(id => {
            const modal = document.getElementById(id);
            if (modal) {
                modal.classList.add('hidden');
            }
        });
        
        if (this.callTimerInterval) {
            clearInterval(this.callTimerInterval);
            this.callTimerInterval = null;
        }
    }

    cleanup() {
        // Stop local stream
        if (this.localStream) {
            this.localStream.getTracks().forEach(track => track.stop());
            this.localStream = null;
        }
        
        // Close peer connection
        if (this.pc) {
            this.pc.close();
            this.pc = null;
        }
        
        // Clear state
        this.currentCallId = null;
        this.currentCallPeer = null;
        this.isInitiator = false;
        this.remoteStream = null;
        this.pendingOffer = null;
        
        // Stop call timer
        if (this.callTimerInterval) {
            clearInterval(this.callTimerInterval);
            this.callTimerInterval = null;
        }
    }
}

// Make classes available globally
window.WebSocketClient = WebSocketClient;
window.VoiceCallManager = VoiceCallManager;
class WebSocketClient {
    constructor(contactName, onMessage, onCallSignal) {
        this.ws = null;
        this.contactName = contactName;
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
            
            // Send initial ping
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

    sendMessage(type, data) {
        if (this.ws && this.ws.readyState === WebSocket.OPEN) {
            const message = {
                type: type,
                ...data,
                timestamp: Date.now()
            };
            
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

// WebRTC Voice Call Manager
class VoiceCallManager {
    constructor(wsClient, username) {
        this.wsClient = wsClient;
        this.username = username;
        this.pc = null;
        this.localStream = null;
        this.remoteStream = null;
        this.currentCallId = null;
        this.currentCallPeer = null;
        this.isInitiator = false;
        
        // Check WebRTC support - Fixed version
        this.RTCPeerConnection = window.RTCPeerConnection || 
                                 window.webkitRTCPeerConnection || 
                                 window.mozRTCPeerConnection;
        
        if (!this.RTCPeerConnection) {
            console.error('WebRTC is not supported in this browser');
        }
        
        // Fixed getUserMedia detection for Firefox
        this.getUserMedia = null;
        if (navigator.mediaDevices && typeof navigator.mediaDevices.getUserMedia === 'function') {
            // Modern API (Firefox, Chrome, Safari, Edge)
            this.getUserMedia = navigator.mediaDevices.getUserMedia.bind(navigator.mediaDevices);
        } else if (navigator.getUserMedia) {
            // Legacy API
            this.getUserMedia = navigator.getUserMedia.bind(navigator);
        } else if (navigator.webkitGetUserMedia) {
            // WebKit legacy
            this.getUserMedia = navigator.webkitGetUserMedia.bind(navigator);
        } else if (navigator.mozGetUserMedia) {
            // Firefox legacy
            this.getUserMedia = navigator.mozGetUserMedia.bind(navigator);
        }
        
        if (!this.getUserMedia) {
            console.error('getUserMedia is not supported in this browser');
        }
        
        // ICE servers configuration (STUN server for NAT traversal)
        this.iceServers = {
            iceServers: [
                { urls: 'stun:stun.l.google.com:19302' },
                { urls: 'stun:stun1.l.google.com:19302' }
            ]
        };
    }
    
    checkWebRTCSupport() {
        // Check RTCPeerConnection
        if (!this.RTCPeerConnection) {
            throw new Error('WebRTC is not supported in your browser. Please use a modern browser (Chrome, Firefox, Safari, or Edge).');
        }
        
        // Check getUserMedia
        if (!this.getUserMedia) {
            throw new Error('Microphone access API is not supported in your browser. Please update to the latest version.');
        }
        
        // Check if we're in a secure context (HTTPS or localhost)
        const isSecureContext = window.isSecureContext || 
                                location.protocol === 'https:' || 
                                ['localhost', '127.0.0.1'].includes(location.hostname);
        
        if (!isSecureContext) {
            throw new Error('Voice calls require HTTPS. Please access the site via HTTPS or localhost.');
        }
    }

    async initiateCall(targetUsername) {
        try {
            // Check WebRTC support first
            this.checkWebRTCSupport();
            
            console.log('Requesting microphone access...');
            
            // Get user media (microphone) - Use the stored function
            this.localStream = await this.getUserMedia({
                audio: true,
                video: false
            });
            
            console.log('Got local stream:', this.localStream);
            
            // Create peer connection
            this.pc = new this.RTCPeerConnection(this.iceServers);
            this.setupPeerConnection();
            
            // Add local stream to peer connection
            this.localStream.getTracks().forEach(track => {
                this.pc.addTrack(track, this.localStream);
            });
            
            // Call backend to initiate call
            const response = await fetch(`/call/initiate/${targetUsername}`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                }
            });
            
            if (!response.ok) {
                const errorData = await response.json().catch(() => ({}));
                throw new Error(errorData.error?.message || 'Failed to initiate call');
            }
            
            const data = await response.json();
            this.currentCallId = data.call_id;
            this.currentCallPeer = targetUsername;
            this.isInitiator = true;
            
            console.log('Call initiated:', data);
            
            // Show calling UI
            this.showCallingUI(targetUsername);
            
        } catch (error) {
            console.error('Failed to initiate call:', error);
            this.endCall();
            
            // Show user-friendly error message
            let errorMessage = error.message;
            
            if (error.name === 'NotAllowedError' || error.name === 'PermissionDeniedError') {
                errorMessage = 'Microphone access denied. Please allow microphone access in your browser settings.';
            } else if (error.name === 'NotFoundError' || error.name === 'DevicesNotFoundError') {
                errorMessage = 'No microphone found. Please connect a microphone and try again.';
            } else if (error.name === 'NotReadableError' || error.name === 'TrackStartError') {
                errorMessage = 'Microphone is already in use by another application.';
            }
            
            alert('Failed to start call: ' + errorMessage);
        }
    }

    async answerCall() {
        try {
            // Check WebRTC support first
            this.checkWebRTCSupport();
            
            console.log('Requesting microphone access...');
            
            // Get user media
            this.localStream = await this.getUserMedia({
                audio: true,
                video: false
            });
            
            console.log('Got local stream:', this.localStream);
            
            // Create peer connection
            this.pc = new this.RTCPeerConnection(this.iceServers);
            this.setupPeerConnection();
            
            // Add local stream
            this.localStream.getTracks().forEach(track => {
                this.pc.addTrack(track, this.localStream);
            });
            
            // Answer the call
            const response = await fetch(`/call/answer/${this.currentCallId}`, {
                method: 'POST'
            });
            
            if (!response.ok) {
                throw new Error('Failed to answer call');
            }
            
            console.log('Call answered');
            
            // Create answer
            const answer = await this.pc.createAnswer();
            await this.pc.setLocalDescription(answer);
            
            // Send answer via WebSocket
            this.wsClient.sendMessage('call_answer', {
                call_id: this.currentCallId,
                to: this.currentCallPeer,
                sdp: answer.sdp
            });
            
            // Show active call UI
            this.showActiveCallUI();
            
        } catch (error) {
            console.error('Failed to answer call:', error);
            this.endCall();
            
            let errorMessage = error.message;
            
            if (error.name === 'NotAllowedError' || error.name === 'PermissionDeniedError') {
                errorMessage = 'Microphone access denied. Please allow microphone access.';
            }
            
            alert('Failed to answer call: ' + errorMessage);
        }
    }

    async rejectCall() {
        try {
            await fetch(`/call/reject/${this.currentCallId}`, {
                method: 'POST'
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
                    method: 'POST'
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
                await this.handleIncomingCall(message.data);
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
        
        // Show notification
        alert(`Call ended by ${message.from}`);
    }

    showIncomingCallUI(caller) {
        const modal = document.getElementById('incoming-call-modal');
        if (!modal) {
            this.createIncomingCallModal(caller);
        } else {
            document.getElementById('caller-name').textContent = caller;
            modal.classList.remove('hidden');
        }
    }

    createIncomingCallModal(caller) {
        const modal = document.createElement('div');
        modal.id = 'incoming-call-modal';
        modal.className = 'fixed inset-0 bg-black/50 flex items-center justify-center z-50';
        modal.innerHTML = `
            <div class="bg-signal-surface rounded-2xl p-8 max-w-sm w-full mx-4 animate-slide-up">
                <div class="text-center">
                    <div class="w-20 h-20 bg-signal-blue rounded-full flex items-center justify-center mx-auto mb-4 animate-pulse">
                        <svg class="w-10 h-10 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 5a2 2 0 012-2h3.28a1 1 0 01.948.684l1.498 4.493a1 1 0 01-.502 1.21l-2.257 1.13a11.042 11.042 0 005.516 5.516l1.13-2.257a1 1 0 011.21-.502l4.493 1.498a1 1 0 01.684.949V19a2 2 0 01-2 2h-1C9.716 21 3 14.284 3 6V5z"></path>
                        </svg>
                    </div>
                    <h3 id="caller-name" class="text-2xl font-bold text-signal-text-main mb-2">${caller}</h3>
                    <p class="text-signal-text-sub mb-8">Incoming voice call</p>
                    <div class="flex gap-4">
                        <button onclick="voiceCall.answerCall()" class="flex-1 bg-green-500 hover:bg-green-600 text-white py-3 rounded-xl transition-all">
                            <svg class="w-6 h-6 inline-block" fill="currentColor" viewBox="0 0 24 24">
                                <path d="M20.01 15.38c-1.23 0-2.42-.2-3.53-.56a.977.977 0 00-1.01.24l-1.57 1.97c-2.83-1.35-5.48-3.9-6.89-6.83l1.95-1.66c.27-.28.35-.67.24-1.02-.37-1.11-.56-2.3-.56-3.53 0-.54-.45-.99-.99-.99H4.19C3.65 3 3 3.24 3 3.99 3 13.28 10.73 21 20.01 21c.71 0 .99-.63.99-1.18v-3.45c0-.54-.45-.99-.99-.99z"></path>
                            </svg>
                        </button>
                        <button onclick="voiceCall.rejectCall()" class="flex-1 bg-red-500 hover:bg-red-600 text-white py-3 rounded-xl transition-all">
                            <svg class="w-6 h-6 inline-block" fill="currentColor" viewBox="0 0 24 24">
                                <path d="M12 9c-1.6 0-3.15.25-4.6.72v3.1c0 .39-.23.74-.56.9-.98.49-1.87 1.12-2.66 1.85-.18.18-.43.28-.7.28-.28 0-.53-.11-.71-.29L.29 13.08c-.18-.17-.29-.42-.29-.7 0-.28.11-.53.29-.71C3.34 8.78 7.46 7 12 7s8.66 1.78 11.71 4.67c.18.18.29.43.29.71 0 .28-.11.53-.29.71l-2.48 2.48c-.18.18-.43.29-.71.29-.27 0-.52-.11-.7-.28-.79-.74-1.69-1.36-2.67-1.85-.33-.16-.56-.5-.56-.9v-3.1C15.15 9.25 13.6 9 12 9z"></path>
                            </svg>
                        </button>
                    </div>
                </div>
            </div>
            <audio id="remote-audio" autoplay></audio>
        `;
        document.body.appendChild(modal);
    }

    showCallingUI(callee) {
        const modal = document.getElementById('calling-modal') || this.createCallingModal(callee);
        document.getElementById('callee-name').textContent = callee;
        modal.classList.remove('hidden');
    }

    createCallingModal(callee) {
        const modal = document.createElement('div');
        modal.id = 'calling-modal';
        modal.className = 'fixed inset-0 bg-black/50 flex items-center justify-center z-50';
        modal.innerHTML = `
            <div class="bg-signal-surface rounded-2xl p-8 max-w-sm w-full mx-4">
                <div class="text-center">
                    <div class="w-20 h-20 bg-signal-blue rounded-full flex items-center justify-center mx-auto mb-4 animate-pulse">
                        <svg class="w-10 h-10 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 5a2 2 0 012-2h3.28a1 1 0 01.948.684l1.498 4.493a1 1 0 01-.502 1.21l-2.257 1.13a11.042 11.042 0 005.516 5.516l1.13-2.257a1 1 0 011.21-.502l4.493 1.498a1 1 0 01.684.949V19a2 2 0 01-2 2h-1C9.716 21 3 14.284 3 6V5z"></path>
                        </svg>
                    </div>
                    <h3 id="callee-name" class="text-2xl font-bold text-signal-text-main mb-2">${callee}</h3>
                    <p class="text-signal-text-sub mb-8">Calling...</p>
                    <button onclick="voiceCall.endCall()" class="bg-red-500 hover:bg-red-600 text-white px-8 py-3 rounded-xl transition-all">
                        End Call
                    </button>
                </div>
            </div>
            <audio id="remote-audio" autoplay></audio>
        `;
        document.body.appendChild(modal);
        return modal;
    }

    showActiveCallUI() {
        const modal = document.getElementById('active-call-modal') || this.createActiveCallModal();
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
        modal.className = 'fixed inset-0 bg-black/80 flex items-center justify-center z-50';
        modal.innerHTML = `
            <div class="bg-signal-surface rounded-2xl p-8 max-w-sm w-full mx-4">
                <div class="text-center">
                    <div class="w-20 h-20 bg-green-500 rounded-full flex items-center justify-center mx-auto mb-4">
                        <svg class="w-10 h-10 text-white" fill="currentColor" viewBox="0 0 24 24">
                            <path d="M20.01 15.38c-1.23 0-2.42-.2-3.53-.56a.977.977 0 00-1.01.24l-1.57 1.97c-2.83-1.35-5.48-3.9-6.89-6.83l1.95-1.66c.27-.28.35-.67.24-1.02-.37-1.11-.56-2.3-.56-3.53 0-.54-.45-.99-.99-.99H4.19C3.65 3 3 3.24 3 3.99 3 13.28 10.73 21 20.01 21c.71 0 .99-.63.99-1.18v-3.45c0-.54-.45-.99-.99-.99z"></path>
                        </svg>
                    </div>
                    <h3 class="text-2xl font-bold text-signal-text-main mb-2">${this.currentCallPeer}</h3>
                    <p id="call-timer" class="text-signal-text-sub mb-8">00:00</p>
                    <button onclick="voiceCall.endCall()" class="bg-red-500 hover:bg-red-600 text-white px-8 py-3 rounded-full transition-all">
                        <svg class="w-6 h-6 inline-block" fill="currentColor" viewBox="0 0 24 24">
                            <path d="M12 9c-1.6 0-3.15.25-4.6.72v3.1c0 .39-.23.74-.56.9-.98.49-1.87 1.12-2.66 1.85-.18.18-.43.28-.7.28-.28 0-.53-.11-.71-.29L.29 13.08c-.18-.17-.29-.42-.29-.7 0-.28.11-.53.29-.71C3.34 8.78 7.46 7 12 7s8.66 1.78 11.71 4.67c.18.18.29.43.29.71 0 .28-.11.53-.29.71l-2.48 2.48c-.18.18-.43.29-.71.29-.27 0-.52-.11-.7-.28-.79-.74-1.69-1.36-2.67-1.85-.33-.16-.56-.5-.56-.9v-3.1C15.15 9.25 13.6 9 12 9z"></path>
                        </svg>
                    </button>
                </div>
            </div>
            <audio id="remote-audio" autoplay></audio>
        `;
        document.body.appendChild(modal);
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
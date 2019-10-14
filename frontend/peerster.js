$(document).ready(function() {
    getAllMessages()
    update()
    timer = setInterval(update, 1000)

    $("#sendMessage").click(function() {
        sendMessage()
    })

    $("#messageInput").keypress(function (e) {
        if (e.which == '13') {
            sendMessage()
        }
    });

    $("#saveNode").click(function(){
        saveNode()
    })

    $("#nodeInput").keypress(function (e) {
        if (e.which == '13') {
            saveNode()
        }
    });
});

function sendMessage() {
    const msg = $('#messageInput').val()
    if (msg === "") {
        return
    }
    $.post("/chat", {msg: msg}) // TODO should add listener to update after done?
    $('#messageInput').val('')
    update()
}

function saveNode() {
    const node = $('#nodeInput').val()
    if (node === "") {
        return
    }
    $.post("/peers", {node: node}) // TODO should add listener to update after done?
    $('#nodeInput').val('')
    update()
}

function getAllMessages() {
    $.get("/fullchat", function(messages){
        if (messages !== null) {
            for (let el of messages) {
                $(".messages").append(el)
            }
            $(".messages").animate({
                scrollTop: $('.messages').prop("scrollHeight")
            }, 1000);
        }
    });
}

function update() {
    $.get("/id", function(id){
        const name = id[0]
        $(".nodeName").text(name)
    })
    $.get("/chat", function(messages){
        if (messages !== null) {
            for (let el of messages) {
                $(".messages").append(el)
            }
            $(".messages").animate({
                scrollTop: $('.messages').prop("scrollHeight")
            }, 1000);
        }
    });
    $.get("/peers", function(peers){
        $(".nodeList").text("")
        if (peers !== null) {
            for (let el of peers) {
                $(".nodeList").append(el)
            }
        }
    });   
}
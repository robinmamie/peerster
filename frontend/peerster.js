$(document).ready(function() {
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
    $.post("/chat", {msg: msg}) // TODO should add listener to update after done?
    $('#messageInput').val('')
    update()
}

function saveNode() {
    const msg = $('#nodeInput').val()
    $.post("/peers", {msg: msg}) // TODO should add listener to update after done?
    $('#nodeInput').val('')
    update()
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
        }
    });
    $.get("/peers", function(peers){
        $(".nodeList").text("")
        if (peers !== null) {
            for (let el of peers) {
                $(".nodeList").append("<p>")
                $(".nodeList").append(el)
                $(".nodeList").append("</p>")
            }
        }
    });   
}
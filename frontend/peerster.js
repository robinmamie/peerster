$(document).ready(function() {
    update()
    timer = setInterval(update, 1000)

    $("#sendMessage").click(function(){
        const msg = $('#messageInput').val()
        $.post("/post", {msg: msg})
        $('#messageInput').val('')
        update()
    })
});

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
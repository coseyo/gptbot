import gradio as gr
import requests


URL = 'http://127.0.0.1:8080'


def handle_error(resp):
    if resp.status_code != 200:
        raise gr.Error(resp.json()['error']['message'])


def create(text):
    resp = requests.post(URL+'/upsert', json=dict(documents=[dict(text=text)]))
    handle_error(resp)
    return 'Created successfully!'


def upload(file):
    with open(file.name) as f:
        resp = requests.post(URL+'/upload', files=dict(file=f))
        handle_error(resp)
        return 'Uploaded successfully!'


def clear():
    resp = requests.post(URL+'/delete', json=dict(document_ids=[]))
    handle_error(resp)
    return 'Cleared successfully!'


def chat(question, history):
    turns = [dict(question=h[0], answer=h[1]) for h in history]
    resp = requests.post(URL+'/chat', json=dict(question=question, history=turns))
    handle_error(resp)
    return resp.json()['answer']


with gr.Blocks() as demo:
    with gr.Tab('Chat Bot'):
        chatbot = gr.Chatbot()
        msg = gr.Textbox(label='Input')

        def user(msg, history):
            question = msg
            return '', history + [[question, None]]

        def bot(history):
            question = history[-1][0]
            answer = chat(question, history[:-1])
            history[-1][1] = answer
            return history

        msg.submit(user, [msg, chatbot], [msg, chatbot], queue=False).then(
            bot, [chatbot], chatbot
        )

    with gr.Tab('Knowledge Base'):
        status = gr.Textbox(label='Status Bar')
        btn = gr.Button(value="Clear All")
        btn.click(clear, inputs=None, outputs=[status])

        with gr.Tab('Document Text'):
            text = gr.Textbox(label='Document Text', lines=8)
            btn = gr.Button(value="Create")
            btn.click(create, inputs=[text], outputs=[status])
        with gr.Tab('Document File'):
            file = gr.File(label='Document File')
            btn = gr.Button(value="Upload")
            btn.click(upload, inputs=[file], outputs=[status])


if __name__ == '__main__':
    demo.launch(server_name='0.0.0.0', server_port=8081, show_error=True)

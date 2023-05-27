package test_k8s

import (
	"context"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	client "k8s.io/client-go/rest"
	v1 "k8s.io/client-go/tools/clientcmd/api/v1"
	"net/url"
)

func TryK8s() {
	u, _ := url.Parse("https://localhost:10000/")
	c, _ := client.NewRESTClient(u, "/api/v2/", client.ClientContentConfig{
		ContentType:  "application/json",
		GroupVersion: v1.SchemeGroupVersion,
		Negotiator:   runtime.NewClientNegotiator(scheme.Codecs.WithoutConversion(), v1.SchemeGroupVersion),
	}, nil, nil)
	clientSet := k8s.New(c)
	_, err := clientSet.AppsV1().StatefulSets("blah").Get(context.Background(), "blah", metav1.GetOptions{
		TypeMeta:        metav1.TypeMeta{},
		ResourceVersion: "",
	})
	fmt.Println(err)
	return
}
